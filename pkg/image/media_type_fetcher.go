/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package image

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const defaultTimeout = 120 * time.Second

// ManifestFetcher abstracts registry access so the check logic can be tested with a mock
type ManifestFetcher interface {
	FetchManifest(ctx context.Context, imageRef string) (*remote.Descriptor, error)
}

// RegistryFetcher implements ManifestFetcher
type RegistryFetcher struct {
	Timeout time.Duration
}

// FetchManifest resolves an image reference (e.g. "quay.io/foo/bar:v1") and returns
// its manifest descriptor, including the MediaType needed for the OCI compatibility check
func (f *RegistryFetcher) FetchManifest(ctx context.Context, imageRef string) (*remote.Descriptor, error) {
	timeout := f.Timeout
	// Protection against setting the timeout to 0
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %w", imageRef, err)
	}

	desc, err := remote.Get(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image manifest for %q: %w", imageRef, err)
	}
	if !desc.MediaType.IsImage() && !desc.MediaType.IsIndex() {
		return nil, fmt.Errorf("unsupported mediatype %q", desc.MediaType)
	}
	return desc, nil
}
