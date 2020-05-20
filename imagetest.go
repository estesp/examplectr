package main

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
)

const (
	defaultContainerdPath = "/run/containerd/containerd.sock"
	defaultImage          = "docker.io/library/busybox:latest"
)

func main() {
	log.SetLevel(log.InfoLevel)
	imageName := defaultImage
	ctx := namespaces.WithNamespace(context.Background(), "examplectr")

	// connect to containerd daemon over UNIX socket
	client, err := containerd.New(defaultContainerdPath)
	if err != nil {
		log.Errorf("error connecting to containerd daemon: %v", err)
		return
	}
	defer client.Close()

	// Cleanup
	err = client.ImageService().Delete(ctx, imageName, images.SynchronousDelete())
	if err != nil && !errdefs.IsNotFound(err) {
		log.Fatal(err)
	}

	testPlatform := platforms.Only(ocispec.Platform{
		OS:           "linux",
		Architecture: "amd64",
	})

	// Pull single platform, do not unpack
	image, err := client.Pull(ctx, imageName, containerd.WithPlatformMatcher(testPlatform))
	if err != nil {
		log.Fatal(err)
	}

	s1, err := image.Usage(ctx, containerd.WithUsageManifestLimit(1))
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Pulled %s: size %d", imageName, s1)

	// Pin image name to specific version for future fetches
	imageName = imageName + "@" + image.Target().Digest.String()
	defer client.ImageService().Delete(ctx, imageName, images.SynchronousDelete())

	// Fetch single platforms, but all manifests pulled
	if _, err := client.Fetch(ctx, imageName, containerd.WithPlatformMatcher(testPlatform), containerd.WithAllMetadata()); err != nil {
		log.Fatal(err)
	}

	if s, err := image.Usage(ctx, containerd.WithUsageManifestLimit(1)); err != nil {
		log.Fatal(err)
	} else if s != s1 {
		log.Fatalf("unexpected usage %d, expected %d", s, s1)
	}

	s2, err := image.Usage(ctx, containerd.WithUsageManifestLimit(0))
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Total size with all manifests: %d", s2)

	if s2 <= s1 {
		log.Fatalf("Expected larger usage counting all manifests: %d <= %d", s2, s1)
	}

	s3, err := image.Usage(ctx, containerd.WithUsageManifestLimit(0), containerd.WithManifestUsage())
	if err != nil {
		log.Fatal(err)
	}

	if s3 < s2 {
		log.Fatalf("Expected larger usage counting all manifest reported sizes: %d <= %d", s3, s2)
	}
	log.Infof("All reported sizes with all manifests: %d", s3)

	// Fetch everything
	if _, err = client.Fetch(ctx, imageName); err != nil {
		log.Fatal(err)
	}
	log.Infof("Fetched all content for %s", imageName)

	s, err := image.Usage(ctx)
	if err != nil {
		log.Fatal(err)
	} else if s != s3 {
		log.Fatalf("Expected actual usage to equal manifest reported usage of %d: got %d", s3, s)
	}
	log.Infof("post-fetch sizes with all manifests: %d", s)

	err = image.Unpack(ctx, containerd.DefaultSnapshotter)
	if err != nil {
		log.Fatal(err)
	}

	if s, err := image.Usage(ctx, containerd.WithSnapshotUsage()); err != nil {
		log.Fatal(err)
	} else if s <= s3 {
		log.Fatalf("Expected actual usage with snapshots to be greater: %d <= %d", s, s3)
	}
}
