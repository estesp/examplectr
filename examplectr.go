package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	log "github.com/sirupsen/logrus"
)

const (
	defaultContainerdPath = "/run/containerd/containerd.sock"
	defaultImage          = "docker.io/library/alpine:latest"
	customCommand         = ""
)

func main() {
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
	fmt.Printf("containerd version (daemon: %s [Revision: %s])", version.Version, version.Revision)

	// let's get an image

	image, err := client.GetImage(ctx, defaultImage)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		image, err = client.Pull(ctx, defaultImage, containerd.WithPullUnpack)
		if err != nil {
			// error pulling the image
			log.Errorf("couldn't pull image: %v", err)
			return
		}
	}

	// create a container
	var container containerd.Container
	if customCommand != "" {
		// the command needs to be overridden in the generated spec
		container, err = client.NewContainer(ctx, "exampleCtr",
			containerd.WithNewSnapshot("exampleCtr", image),
			containerd.WithNewSpec(oci.WithImageConfig(image),
				oci.WithProcessArgs(strings.Split(customCommand, " ")...)))
	} else {
		container, err = client.NewContainer(ctx, "exampleCtr",
			containerd.WithNewSpec(oci.WithImageConfig(image)),
			containerd.WithNewSnapshot("exampleCtr", image))
	}
	if err != nil {
		log.Errorf("error creating container: %v", err)
		return
	}

	stdouterr := bytes.NewBuffer(nil)
	task, err := container.NewTask(ctx, cio.NewIO(bytes.NewBuffer(nil), stdouterr, stdouterr))
	if err != nil {
		log.Errorf("error creating task: %v", err)
		return
	}
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		log.Errorf("error starting task: %v", err)
		return
	}
	return
}

// stopContainer will stop/kill a container (specifically, the tasks [processes]
// running in the container)
func stopContainer(ctx context.Context, client containerd.Client, name string) error {
	container, err := client.LoadContainer(ctx, name)
	if err != nil {
		return err
	}
	if err = stopTask(ctx, container); err != nil {
		// ignore if the error is that the process had already exited:
		if !strings.Contains(err.Error(), "not found") {
			return err
		}
	}
	return nil
}

// deleteContainer will remove a container
func deleteContainer(ctx context.Context, client containerd.Client, name string) error {
	container, err := client.LoadContainer(ctx, name)
	if err != nil {
		return err
	}
	return container.Delete(ctx, containerd.WithSnapshotCleanup)
}

// common code for task stop/kill using the containerd gRPC API
func stopTask(ctx context.Context, ctr containerd.Container) error {
	task, err := ctr.Task(ctx, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "no running task") {
			return err
		}
		//nothing to do; no task running
		return nil
	}
	status, err := task.Status(ctx)
	switch status.Status {
	case containerd.Stopped:
		_, err := task.Delete(ctx)
		if err != nil {
			return err
		}
	case containerd.Running:
		statusC, err := task.Wait(ctx)
		if err != nil {
			log.Errorf("container %q: error during wait: %v", ctr.ID(), err)
		}
		if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
			task.Delete(ctx)
			return err
		}
		status := <-statusC
		code, _, err := status.Result()
		if err != nil {
			log.Errorf("container %q: error getting task result code: %v", ctr.ID(), err)
		}
		if code != 0 {
			log.Debugf("%s: exited container process: code: %d", ctr.ID(), status)
		}
		_, err = task.Delete(ctx)
		if err != nil {
			return err
		}
	case containerd.Paused:
		return fmt.Errorf("Can't stop a paused container; unpause first")
	}
	return nil
}
