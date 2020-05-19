package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	log "github.com/sirupsen/logrus"
)

const (
	defaultContainerdPath = "/run/containerd/containerd.sock"
	defaultImage          = "docker.io/library/alpine:latest"
)

func main() {

	var (
		imageName string
		command   string
	)

	switch len(os.Args) {
	case 2:
		imageName = os.Args[1]
	case 3:
		imageName = os.Args[1]
		command = os.Args[2]
	default:
		imageName = defaultImage
	}

	// connect to containerd daemon over UNIX socket
	client, err := containerd.New(defaultContainerdPath)
	if err != nil {
		log.Errorf("error connecting to containerd daemon: %v", err)
		return
	}

	// set up a namespace
	ctx := namespaces.WithNamespace(context.Background(), "examplectr")

	version, err := client.Version(ctx)
	if err != nil {
		log.Errorf("error retrieving containerd version: %v", err)
		return
	}
	fmt.Printf("containerd version (daemon: %s [Revision: %s])\n", version.Version, version.Revision)

	// let's get an image
	image, err := client.GetImage(ctx, imageName)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		image, err = client.Pull(ctx, imageName, containerd.WithPullUnpack)
		if err != nil {
			// error pulling the image
			log.Errorf("couldn't pull image %s: %v", imageName, err)
			return
		}
	}

	// create a container
	var container containerd.Container
	if command != "" {
		// the command needs to be overridden in the generated spec
		container, err = client.NewContainer(ctx, "exampleCtr",
			containerd.WithNewSnapshot("exampleCtr", image),
			containerd.WithNewSpec(oci.WithImageConfig(image),
				oci.WithProcessArgs(strings.Split(command, " ")...)))
	} else {
		container, err = client.NewContainer(ctx, "exampleCtr",
			containerd.WithNewSpec(oci.WithImageConfig(image)),
			containerd.WithNewSnapshot("exampleCtr", image))
	}
	if err != nil {
		log.Errorf("error creating container: %v", err)
		return
	}

	// create a task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		log.Errorf("error creating task: %v", err)
		return
	}

	// start the task
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		log.Errorf("error starting task: %v", err)
		return
	}
}
