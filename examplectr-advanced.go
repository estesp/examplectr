package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/estesp/examplectr/idtools"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	defaultContainerdPath = "/run/containerd2/containerd.sock"
	defaultImage          = "docker.io/library/alpine:latest"
)

// simple client object for executing containers
type cc struct {
	ctx        context.Context
	client     *containerd.Client
	image      string
	name       string
	command    string
	idMappings *idtools.IDMappings
}

func main() {
	var (
		username  string
		imageName string
		command   string
	)

	// usage: ./examplectr <user> [<image> <command>]
	switch len(os.Args) {
	case 1:
		log.Errorf("Must provide at least a username for a container")
		os.Exit(-1)
	case 2:
		username = os.Args[1]
		imageName = defaultImage
	case 3:
		username = os.Args[1]
		imageName = os.Args[2]
	case 4:
		username = os.Args[1]
		imageName = os.Args[2]
		command = os.Args[3]
	default:
		username = os.Args[1]
		imageName = os.Args[2]
		command = strings.Join(os.Args[3:], " ")
	}

	// check for id mappings for user namespaces
	idMappings, err := idtools.NewIDMappings(username, username)
	if err != nil {
		log.Errorf("error finding ID mappings for user %s: %v", username, err)
		os.Exit(-1)
	}

	// connect to containerd daemon over UNIX socket
	client, err := containerd.New(defaultContainerdPath)
	if err != nil {
		log.Errorf("error connecting to containerd daemon: %v", err)
		os.Exit(-1)
	}

	// set up a namespace
	ctx := namespaces.WithNamespace(context.Background(), "examplectr")

	cclient := &cc{
		ctx:        ctx,
		client:     client,
		idMappings: idMappings,
		image:      imageName,
		name:       fmt.Sprintf("exampleCtr-%d", os.Getpid()),
		command:    command,
	}

	cclient.printVersion()

	exitStatus, err := cclient.runContainer()
	if err != nil {
		log.Errorf("failed to run container: %v", err)
	}
	if exitStatus.Error() != nil {
		log.Errorf("container exited with error: %v", exitStatus.Error())
	}
	os.Exit(int(exitStatus.ExitCode()))
}

func (c *cc) runContainer() (containerd.ExitStatus, error) {
	// let's get an image
	image, err := c.client.GetImage(c.ctx, c.image)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		image, err = c.client.Pull(c.ctx, c.image, containerd.WithPullUnpack)
		if err != nil {
			return containerd.ExitStatus{}, errors.Wrapf(err, "couldn't pull image %s", c.image)
		}
	}

	// create a container
	container, err := c.newContainer(image)
	if err != nil {
		return containerd.ExitStatus{}, errors.Wrap(err, "error creating container")
	}
	// if there is a command, we'll do a full lifecycle including cleanup
	if c.command != "" {
		defer container.Delete(c.ctx, containerd.WithSnapshotCleanup)
	}

	// create a task
	newTaskOpts := []containerd.NewTaskOpts{}
	stdouterr := bytes.NewBuffer(nil)
	if c.idMappings != nil {
		rootPair := c.idMappings.RootPair()
		newTaskOpts = append(newTaskOpts, func(_ context.Context, client *containerd.Client, r *containerd.TaskInfo) error {
			//r.Options = &runctypes.CreateOptions{
			//	IoUid: uint32(rootPair.UID),
			//	IoGid: uint32(rootPair.GID),
			//}
			return nil
		})
	}
	task, err := container.NewTask(c.ctx, cio.NewIO(bytes.NewBuffer(nil), stdouterr, stdouterr), newTaskOpts...)
	if err != nil {
		return containerd.ExitStatus{}, errors.Wrap(err, "error creating task")
	}
	if c.command != "" {
		defer task.Delete(c.ctx)
	}

	// if an explicit command was provided, then wait on the task
	var statusC <-chan containerd.ExitStatus
	if c.command != "" {
		statusC, err = task.Wait(c.ctx)
		if err != nil {
			return containerd.ExitStatus{}, errors.Wrap(err, "error waiting on task")
		}
	}

	// start the task
	if err := task.Start(c.ctx); err != nil {
		task.Delete(c.ctx)
		return containerd.ExitStatus{}, errors.Wrap(err, "error starting task")
	}

	if c.command != "" {
		exitStatus := <-statusC
		fmt.Printf("%s", string(stdouterr.Bytes()))
		return exitStatus, nil
	}
	return containerd.ExitStatus{}, nil
}

func (c *cc) newContainer(image containerd.Image) (containerd.Container, error) {
	newOpts := []containerd.NewContainerOpts{}
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
	}
	if c.command != "" {
		specOpts = append(specOpts, oci.WithProcessArgs(strings.Split(c.command, " ")...))
	}

	if c.idMappings != nil {
		rootPair := c.idMappings.RootPair()
		uidMaps := c.idMappings.UIDs()
		// use user namespaces for this container
		specOpts = append(specOpts, oci.WithUserNamespace(uint32(uidMaps[0].ContainerID),
			uint32(uidMaps[0].HostID), uint32(uidMaps[0].Size)))
		newOpts = append(newOpts, containerd.WithRemappedSnapshot(c.name, image,
			uint32(rootPair.UID), uint32(rootPair.GID)))
	} else {
		newOpts = append(newOpts, containerd.WithNewSnapshot(c.name, image))
	}
	// allow for local bind mounts with our helper function
	specOpts = append(specOpts, withMounts())
	newOpts = append(newOpts, containerd.WithNewSpec(specOpts...))

	return c.client.NewContainer(c.ctx, c.name, newOpts...)
}

func (c *cc) printVersion() {
	version, err := c.client.Version(c.ctx)
	if err != nil {
		log.Errorf("error retrieving containerd version: %v", err)
		return
	}
	fmt.Printf("containerd version (daemon: %s [Revision: %s])\n", version.Version, version.Revision)
}
