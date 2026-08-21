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
	"log/slog"

	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/konflux-ci/operator-foundry/pkg/ocp"
)

// first OCP version that supports OCI MediaType natively
const ociNativeMinOCPVersion = "4.21"

// MediaTypeCheckResult holds the outcome of the MediaType and OCP version compatibility check
type MediaTypesCheckResult struct {
	Passed               bool
	WrongMediaTypeImages []string
	BrokenImages         []string
}

type MediaTypeCheckResult struct {
	Passed              bool
	WrongMediaTypeImage string
	BrokenImage         string
}

func CheckRelatedImageMediaType(ctx context.Context, relatedImage string, fetcher ManifestFetcher, ocpVersion string) MediaTypeCheckResult {
	imageMediaTypeDescriptor, err := fetcher.FetchManifest(ctx, relatedImage)
	if err != nil {
		slog.Warn(
			"failed to fetch media type from related image",
			"error", err,
			"ocpVersion", ocpVersion,
			"relatedImage", relatedImage,
		)
		return MediaTypeCheckResult{
			Passed:      false,
			BrokenImage: relatedImage,
		}
	}

	switch imageMediaTypeDescriptor.MediaType {

	// application/vnd.docker.distribution.manifest.v2+json
	// Docker V2 — compatible, nothing to do
	case types.DockerManifestSchema2:
		return MediaTypeCheckResult{Passed: true}

	// application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json
	// OCI — incompatible with OCP < 4.21
	case types.OCIManifestSchema1, types.OCIImageIndex:
		slog.Warn(
			"OCI media type is not supported by ocp version < 4.21, rebuild or re-push the image using Docker V2 manifests",
			"ocpVersion", ocpVersion,
			"relatedImage", relatedImage,
			"MediaType", imageMediaTypeDescriptor.MediaType,
		)
		return MediaTypeCheckResult{Passed: false, WrongMediaTypeImage: relatedImage}

	// application/vnd.docker.distribution.manifest.list.v2+json
	// Multi-arch — need to check each inner manifest
	case types.DockerManifestList:
		idx, err := imageMediaTypeDescriptor.ImageIndex()
		if err != nil {
			slog.Warn(
				"failed to read manifest list as index image",
				"error", err,
				"ocpVersion", ocpVersion,
				"relatedImage", relatedImage,
				"MediaType", imageMediaTypeDescriptor.MediaType,
			)
			return MediaTypeCheckResult{Passed: false, BrokenImage: relatedImage}
		}
		idxManifest, err := idx.IndexManifest()
		if err != nil {
			slog.Warn(
				"failed to parse index manifest",
				"error", err,
				"ocpVersion", ocpVersion,
				"relatedImage", relatedImage,
				"MediaType", imageMediaTypeDescriptor.MediaType,
			)
			return MediaTypeCheckResult{Passed: false, BrokenImage: relatedImage}
		}
		for _, manifest := range idxManifest.Manifests {
			if manifest.MediaType != types.DockerManifestSchema2 {
				slog.Warn(
					"inner manifest media type is incompatible with ocp version < 4.21, rebuild or re-push the image using Docker V2 manifests",
					"ocpVersion", ocpVersion,
					"relatedImage", relatedImage,
					"MediaType", manifest.MediaType,
				)
				return MediaTypeCheckResult{Passed: false, WrongMediaTypeImage: relatedImage}
			}
		}
	default:
		slog.Warn("unexpected media type, treating as incompatible",
			"ocpVersion", ocpVersion,
			"relatedImage", relatedImage,
			"MediaType", imageMediaTypeDescriptor.MediaType,
		)
		return MediaTypeCheckResult{Passed: false, WrongMediaTypeImage: relatedImage}
	}
	return MediaTypeCheckResult{Passed: true}
}

// CheckRelatedImagesMediaType checks whether all related images
// are compliant with the constraint that OCI MediaType is not present for OCP < v4.21.
// If the ocp version is >= v4.21, this check is skipped, otherwise we pull the MediaType,
// check the values, and if there is DockerManifestList we also check its Image manifests
//
// Returns (nil, err) if the OCP version is malformed or comparison fails.
// Returns a MediaTypeCheckResult with Passed=true if all images are compliant or
// the OCP version >= 4.21. Returns a MediaTypeCheckResult with Passed=false and
// WrongMediaTypeImages populated for every non-compliant image.
func CheckRelatedImagesMediaType(
	ctx context.Context, ocpVersion string, relatedImages []string, fetcher ManifestFetcher,
) (*MediaTypesCheckResult, error) {
	// skip the rest of the check if the ocpVersion>=4.21
	gte, err := ocp.OCPVersionGTE(ocpVersion, ociNativeMinOCPVersion)
	if err != nil {
		return nil, err
	}
	if gte {
		slog.Info("ocp version supports OCI media types natively, skipping check",
			"ocpVersion", ocpVersion,
		)
		return &MediaTypesCheckResult{Passed: true}, nil
	}

	var wrongMediaTypeImages []string
	var brokenImages []string

	for _, relatedImage := range relatedImages {
		var result MediaTypeCheckResult = CheckRelatedImageMediaType(
			ctx, relatedImage, fetcher, ocpVersion)
		if result.BrokenImage != "" {
			brokenImages = append(brokenImages, result.BrokenImage)
		}
		if result.WrongMediaTypeImage != "" {
			wrongMediaTypeImages = append(wrongMediaTypeImages, result.WrongMediaTypeImage)
		}
	}
	if len(wrongMediaTypeImages) > 0 || len(brokenImages) > 0 {
		return &MediaTypesCheckResult{Passed: false, WrongMediaTypeImages: wrongMediaTypeImages, BrokenImages: brokenImages}, nil
	}

	return &MediaTypesCheckResult{Passed: true}, nil
}
