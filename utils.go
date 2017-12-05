package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	log "github.com/sirupsen/logrus"
)

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

// withMounts
func withMounts() oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		mounts, err := os.Open("mounts")
		if err != nil {
			// don't add any mounts if any kind of error
			return nil
		}
		defer mounts.Close()
		scanner := bufio.NewScanner(mounts)
		for scanner.Scan() {
			line := scanner.Text()
			// line format = dest:type:source
			//   example "/etc/hosts:bind:/etc/hosts"
			mountinfo := strings.Split(line, ":")
			if len(mountinfo) != 3 {
				continue
			}
			s.Mounts = append(s.Mounts, specs.Mount{
				Destination: mountinfo[0],
				Type:        mountinfo[1],
				Source:      mountinfo[2],
				Options:     []string{"rbind"},
			})
		}
		return nil
	}
}
