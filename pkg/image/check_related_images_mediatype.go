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
type MediaTypeCheckResult struct {
	Passed              bool
	WrongMediaTypeImages []string
	BrokenImages        []string
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
) (*MediaTypeCheckResult, error) {
	// skip the rest of the check if the ocpVersion>=4.21
	gte, err := ocp.OCPVersionGTE(ocpVersion, ociNativeMinOCPVersion)
	if err != nil {
		return nil, err
	}
	if gte {
		slog.Info("ocp version supports OCI media types natively, skipping check",
			"ocpVersion", ocpVersion,
		)
		return &MediaTypeCheckResult{Passed: true}, nil
	}

	var wrongMediaTypeImages []string
	var brokenImages []string

	for _, relatedImage := range relatedImages {
		imageMediaTypeDescriptor, err := fetcher.FetchManifest(ctx, relatedImage)
		if err != nil {
			slog.Warn(
				"failed to fetch media type from related image",
				"error", err,
				"relatedImage", relatedImage,
			)
			brokenImages = append(brokenImages, relatedImage)
			// continue with the rest of relatedImages, so the user is aware of all the issues
			continue
		}

		switch imageMediaTypeDescriptor.MediaType {

		// application/vnd.docker.distribution.manifest.v2+json
		// Docker V2 — compatible, nothing to do
		case types.DockerManifestSchema2:
			continue

		// application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json
		// OCI — incompatible with OCP < 4.21
		case types.OCIManifestSchema1, types.OCIImageIndex:
			slog.Warn(
				"OCI media type is not supported by ocp version < 4.21, rebuild or re-push the image using Docker V2 manifests",
				"ocpVersion", ocpVersion,
				"relatedImage", relatedImage,
				"MediaType", imageMediaTypeDescriptor.MediaType,
			)
			wrongMediaTypeImages = append(wrongMediaTypeImages, relatedImage)
			continue

		// application/vnd.docker.distribution.manifest.list.v2+json
		// Multi-arch — need to check each inner manifest
		case types.DockerManifestList:
			idx, err := imageMediaTypeDescriptor.ImageIndex()
			if err != nil {
				slog.Warn(
					"failed to read manifest list as index image",
					"error", err, "relatedImage", relatedImage,
				)
				brokenImages = append(brokenImages, relatedImage)
				continue
			}
			idxManifest, err := idx.IndexManifest()
			if err != nil {
				slog.Warn(
					"failed to parse index manifest",
					"error", err, "relatedImage", relatedImage,
				)
				brokenImages = append(brokenImages, relatedImage)
				continue
			}
			for _, manifest := range idxManifest.Manifests {
				if manifest.MediaType != types.DockerManifestSchema2 {
					slog.Warn(
						"inner manifest media type is incompatible with ocp version < 4.21, rebuild or re-push the image using Docker V2 manifests",
						"ocpVersion", ocpVersion,
						"relatedImage", relatedImage,
						"MediaType", manifest.MediaType,
					)
					wrongMediaTypeImages = append(wrongMediaTypeImages, relatedImage)
					break
				}
			}
		default:
			slog.Warn("unexpected media type, treating as incompatible",
				"relatedImage", relatedImage,
				"MediaType", imageMediaTypeDescriptor.MediaType,
			)
			wrongMediaTypeImages = append(wrongMediaTypeImages, relatedImage)
			continue
		}

	}
	if len(wrongMediaTypeImages) > 0 || len(brokenImages) > 0 {
		return &MediaTypeCheckResult{Passed: false, WrongMediaTypeImages: wrongMediaTypeImages, BrokenImages: brokenImages}, nil
	}

	return &MediaTypeCheckResult{Passed: true}, nil
}
