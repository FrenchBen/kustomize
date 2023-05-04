// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package localizer

import (
	"context"
	"fmt"
	"log"
	"time"

	OciClient "github.com/fluxcd/pkg/oci/client"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"sigs.k8s.io/kustomize/api/internal/localizer"
	"sigs.k8s.io/kustomize/api/internal/oci"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type SourceOCIProvider struct {
	oci.SourceOCIProvider
}

// Run executes `kustomize localize` on fSys given the `localize` arguments and
// returns the path to the created newDir.
func Run(fSys filesys.FileSystem, target, scope, newDir string) (string, error) {
	dst, err := localizer.Run(target, scope, newDir, fSys)
	return dst, errors.Wrap(err)
}

// Pull executes `kustomize localize` on OCI artifacts
// returns the path to the created destination
func Pull(target, destination string, provider SourceOCIProvider, creds string) (string, error) {
	output := destination
	ociURL, err := OciClient.ParseArtifactURL(target)
	if err != nil {
		return "", err
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ociClient := OciClient.NewLocalClient()

	if provider.String() == v1beta2.GenericOCIProvider && creds != "" {
		log.Println("logging in to registry with credentials")
		if err := ociClient.LoginWithCredentials(creds); err != nil {
			return "", fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	if provider.String() != v1beta2.GenericOCIProvider {
		log.Println("logging in to registry with provider credentials")
		ociProvider, err := provider.ToOCIProvider()
		if err != nil {
			return "", fmt.Errorf("provider not supported: %w", err)
		}

		if err := ociClient.LoginWithProvider(ctx, ociURL, ociProvider); err != nil {
			return "", fmt.Errorf("error during login with provider: %w", err)
		}
	}

	log.Printf("pulling artifact from %s", ociURL)

	meta, err := ociClient.Pull(ctx, ociURL, output)
	if err != nil {
		return "", err
	}

	log.Printf("source %s", meta.Source)
	log.Printf("revision %s", meta.Revision)
	log.Printf("digest %s", meta.Digest)
	log.Printf("artifact content extracted to %s", output)
	return output, nil
}
