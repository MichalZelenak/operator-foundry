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
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// mockFetcher implements ManifestFetcher for testing.
type mockFetcher struct {
	results map[string]*remote.Descriptor
	errors  map[string]error
}

func (m *mockFetcher) FetchManifest(_ context.Context, imageRef string) (*remote.Descriptor, error) {
	if err, ok := m.errors[imageRef]; ok {
		return nil, err
	}
	if desc, ok := m.results[imageRef]; ok {
		return desc, nil
	}
	return nil, fmt.Errorf("unexpected image ref in mock: %q", imageRef)
}

// makeDescriptor creates a remote.Descriptor with the given MediaType.
func makeDescriptor(mediaType types.MediaType) *remote.Descriptor {
	return &remote.Descriptor{
		Descriptor: v1.Descriptor{
			MediaType: mediaType,
		},
	}
}

// makeManifestListDescriptor creates a remote.Descriptor for a DockerManifestList
// with inner manifests of the given MediaTypes.
func makeManifestListDescriptor(innerMediaTypes ...types.MediaType) *remote.Descriptor {
	var manifests []v1.Descriptor
	for i, mt := range innerMediaTypes {
		manifests = append(manifests, v1.Descriptor{
			MediaType: mt,
			Size:      100,
			Digest: v1.Hash{
				Algorithm: "sha256",
				Hex:       fmt.Sprintf("%064d", i),
			},
		})
	}
	idx := v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.DockerManifestList,
		Manifests:     manifests,
	}
	raw, _ := json.Marshal(idx)

	return &remote.Descriptor{
		Descriptor: v1.Descriptor{
			MediaType: types.DockerManifestList,
		},
		Manifest: raw,
	}
}

// ── OCP version >= 4.21 — skip check ────────────────────────────────────────

func TestCheckRelatedImagesMediaType_OCPVersionGTE421_SkipsCheck(t *testing.T) {
	fetcher := &mockFetcher{}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.21", []string{"quay.io/example/img:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("got Passed=false, want true for OCP >= 4.21")
	}
}

func TestCheckRelatedImagesMediaType_OCPVersion500_SkipsCheck(t *testing.T) {
	fetcher := &mockFetcher{}
	result, err := CheckRelatedImagesMediaType(context.Background(), "5.0", []string{"quay.io/example/img:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("got Passed=false, want true for OCP >= 4.21")
	}
}

// ── Invalid OCP version — error ─────────────────────────────────────────────

func TestCheckRelatedImagesMediaType_InvalidOCPVersion_ReturnsError(t *testing.T) {
	fetcher := &mockFetcher{}
	_, err := CheckRelatedImagesMediaType(context.Background(), "invalid", []string{"quay.io/example/img:v1"}, fetcher)
	if err == nil {
		t.Fatal("expected error for invalid OCP version, got nil")
	}
}

// ── Empty image list — pass ─────────────────────────────────────────────────

func TestCheckRelatedImagesMediaType_EmptyImageList_Passes(t *testing.T) {
	fetcher := &mockFetcher{}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("got Passed=false, want true for empty image list")
	}
}

// ── Docker V2 single-arch — pass ────────────────────────────────────────────

func TestCheckRelatedImagesMediaType_DockerV2SingleArch_Passes(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/img:v1": makeDescriptor(types.DockerManifestSchema2),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/img:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("got Passed=false, want true for Docker V2 manifest")
	}
}

// ── OCI single-arch — fail ──────────────────────────────────────────────────

func TestCheckRelatedImagesMediaType_OCIManifest_Fails(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/oci:v1": makeDescriptor(types.OCIManifestSchema1),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/oci:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false for OCI manifest")
	}
	if len(result.WrongMediaTypeImages) != 1 || result.WrongMediaTypeImages[0] != "quay.io/example/oci:v1" {
		t.Errorf("got WrongMediaTypeImages=%v, want [quay.io/example/oci:v1]", result.WrongMediaTypeImages)
	}
}

// ── OCI image index — fail ──────────────────────────────────────────────────

func TestCheckRelatedImagesMediaType_OCIImageIndex_Fails(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/idx:v1": makeDescriptor(types.OCIImageIndex),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/idx:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false for OCI image index")
	}
	if len(result.WrongMediaTypeImages) != 1 || result.WrongMediaTypeImages[0] != "quay.io/example/idx:v1" {
		t.Errorf("got WrongMediaTypeImages=%v, want [quay.io/example/idx:v1]", result.WrongMediaTypeImages)
	}
}

// ── Docker manifest list with all Docker V2 inner — pass ────────────────────

func TestCheckRelatedImagesMediaType_DockerManifestList_AllDockerV2Inner_Passes(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/multi:v1": makeManifestListDescriptor(
				types.DockerManifestSchema2,
				types.DockerManifestSchema2,
			),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/multi:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("got Passed=false, want true for manifest list with all Docker V2 inner manifests")
	}
}

// ── Docker manifest list with OCI inner — fail ──────────────────────────────

func TestCheckRelatedImagesMediaType_DockerManifestList_OCIInner_Fails(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/mixed:v1": makeManifestListDescriptor(
				types.DockerManifestSchema2,
				types.OCIManifestSchema1,
			),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/mixed:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false for manifest list with OCI inner manifest")
	}
	if len(result.WrongMediaTypeImages) != 1 || result.WrongMediaTypeImages[0] != "quay.io/example/mixed:v1" {
		t.Errorf("got WrongMediaTypeImages=%v, want [quay.io/example/mixed:v1]", result.WrongMediaTypeImages)
	}
}

// ── Fetch error — treated as broken ─────────────────────────────────────────

func TestCheckRelatedImagesMediaType_FetchError_FailsGracefully(t *testing.T) {
	fetcher := &mockFetcher{
		errors: map[string]error{
			"quay.io/example/broken:v1": fmt.Errorf("connection refused"),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/broken:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false when fetch fails")
	}
	if len(result.WrongMediaTypeImages) != 0 {
		t.Errorf("got WrongMediaTypeImages=%v, want empty", result.WrongMediaTypeImages)
	}
	if len(result.BrokenImages) != 1 || result.BrokenImages[0] != "quay.io/example/broken:v1" {
		t.Errorf("got BrokenImages=%v, want [quay.io/example/broken:v1]", result.BrokenImages)
	}
}

// ── Mixed results — all failures collected ──────────────────────────────────

func TestCheckRelatedImagesMediaType_MixedResults_CollectsAllFailures(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/good:v1":  makeDescriptor(types.DockerManifestSchema2),
			"quay.io/example/bad:v1":   makeDescriptor(types.OCIManifestSchema1),
			"quay.io/example/multi:v1": makeManifestListDescriptor(types.DockerManifestSchema2, types.OCIManifestSchema1),
		},
		errors: map[string]error{
			"quay.io/example/down:v1": fmt.Errorf("timeout"),
		},
	}
	images := []string{
		"quay.io/example/good:v1",
		"quay.io/example/bad:v1",
		"quay.io/example/down:v1",
		"quay.io/example/multi:v1",
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", images, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false")
	}
	if len(result.WrongMediaTypeImages) != 2 {
		t.Errorf("got WrongMediaTypeImages=%v, want 2 (bad + mixed)", result.WrongMediaTypeImages)
	}
	if len(result.BrokenImages) != 1 || result.BrokenImages[0] != "quay.io/example/down:v1" {
		t.Errorf("got BrokenImages=%v, want [quay.io/example/down:v1]", result.BrokenImages)
	}
}

// ── Unexpected media type — treated as failed ──────────────────────────────

func TestCheckRelatedImagesMediaType_UnexpectedMediaType_Failed(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/weird:v1": makeDescriptor(types.MediaType("application/vnd.unknown")),
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/weird:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false for unexpected media type")
	}
	if len(result.WrongMediaTypeImages) != 1 || result.WrongMediaTypeImages[0] != "quay.io/example/weird:v1" {
		t.Errorf("got WrongMediaTypeImages=%v, want [quay.io/example/weird:v1]", result.WrongMediaTypeImages)
	}
	if len(result.BrokenImages) != 0 {
		t.Errorf("got BrokenImages=%v, want empty", result.BrokenImages)
	}
}

// ── Manifest list with invalid manifest bytes — treated as broken ───────────

func TestCheckRelatedImagesMediaType_ManifestListInvalidBody_Broken(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/broken-list:v1": {
				Descriptor: v1.Descriptor{
					MediaType: types.DockerManifestList,
				},
				Manifest: []byte("not valid json"),
			},
		},
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", []string{"quay.io/example/broken-list:v1"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("got Passed=true, want false for broken manifest list")
	}
	if len(result.WrongMediaTypeImages) != 0 {
		t.Errorf("got WrongMediaTypeImages=%v, want empty", result.WrongMediaTypeImages)
	}
	if len(result.BrokenImages) != 1 || result.BrokenImages[0] != "quay.io/example/broken-list:v1" {
		t.Errorf("got BrokenImages=%v, want [quay.io/example/broken-list:v1]", result.BrokenImages)
	}
}

// ── Multiple Docker V2 images — all pass ────────────────────────────────────

func TestCheckRelatedImagesMediaType_MultipleDockerV2_AllPass(t *testing.T) {
	fetcher := &mockFetcher{
		results: map[string]*remote.Descriptor{
			"quay.io/example/a:v1": makeDescriptor(types.DockerManifestSchema2),
			"quay.io/example/b:v1": makeDescriptor(types.DockerManifestSchema2),
			"quay.io/example/c:v1": makeDescriptor(types.DockerManifestSchema2),
		},
	}
	images := []string{
		"quay.io/example/a:v1",
		"quay.io/example/b:v1",
		"quay.io/example/c:v1",
	}
	result, err := CheckRelatedImagesMediaType(context.Background(), "4.20", images, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("got Passed=false, want true for all Docker V2 images")
	}
	if len(result.WrongMediaTypeImages) != 0 {
		t.Errorf("got WrongMediaTypeImages=%v, want empty", result.WrongMediaTypeImages)
	}
}
