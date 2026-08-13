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
	"strings"
	"testing"
	"time"
)

func TestRegistryFetcher_InvalidImageReference_ReturnsError(t *testing.T) {
	fetcher := &RegistryFetcher{Timeout: 5 * time.Second}
	_, err := fetcher.FetchManifest(context.Background(), ":::invalid")
	if err == nil {
		t.Fatal("expected error for invalid image reference, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse image reference") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestRegistryFetcher_CancelledContext_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetcher := &RegistryFetcher{Timeout: 5 * time.Second}
	_, err := fetcher.FetchManifest(ctx, "quay.io/example/img:v1")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestRegistryFetcher_ZeroTimeout_UsesDefault(t *testing.T) {
	fetcher := &RegistryFetcher{}
	_, err := fetcher.FetchManifest(context.Background(), ":::invalid")
	if err == nil {
		t.Fatal("expected error for invalid image reference, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse image reference") {
		t.Errorf("expected parse error (not timeout), got: %v", err)
	}
}
