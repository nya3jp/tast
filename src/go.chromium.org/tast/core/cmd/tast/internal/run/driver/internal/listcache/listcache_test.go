// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package listcache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/internal/protocol"
)

var (
	params = &protocol.RunnerInitParams{BundleGlob: "/usr/local/libexec/tast/bundles/local/cros"}
	req    = &protocol.ListEntitiesRequest{Features: &protocol.Features{CheckDeps: true}}
	res    = &protocol.ListEntitiesResponse{Entities: []*protocol.ResolvedEntity{
		{Entity: &protocol.Entity{Type: protocol.EntityType_TEST, Name: "pkg.Test"}},
	}}
)

// setUp writes fake executables to a temporary directory, and returns their
// paths and the cache directory.
func setUp(t *testing.T) (runner, bundle, dir string) {
	t.Helper()
	td := t.TempDir()
	runner = filepath.Join(td, "local_test_runner")
	bundle = filepath.Join(td, "cros")
	for _, p := range []string{runner, bundle} {
		if err := os.WriteFile(p, []byte(filepath.Base(p)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return runner, bundle, filepath.Join(td, "cache")
}

func TestCacheLoadStore(t *testing.T) {
	ctx := t.Context()
	runner, bundle, dir := setUp(t)

	c := New(dir, runner, bundle)
	if got := c.Load(ctx, params, req); got != nil {
		t.Fatalf("Load() before Store() = %v; want nil", got)
	}
	c.Store(ctx, params, req, res)

	// A cache for the same executables, e.g. in a later run, hits.
	got := New(dir, runner, bundle).Load(ctx, params, req)
	if diff := cmp.Diff(got, res, protocmp.Transform()); diff != "" {
		t.Errorf("Load() mismatch (-got +want):\n%s", diff)
	}
}

func TestCacheKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		changed string // executable changed after storing; "runner" or "bundle"
		params  *protocol.RunnerInitParams
		req     *protocol.ListEntitiesRequest
	}{
		{name: "runner_changed", changed: "runner", params: params, req: req},
		{name: "bundle_changed", changed: "bundle", params: params, req: req},
		{name: "glob_changed", params: &protocol.RunnerInitParams{BundleGlob: "/other"}, req: req},
		{name: "features_changed", params: params, req: &protocol.ListEntitiesRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			runner, bundle, dir := setUp(t)
			New(dir, runner, bundle).Store(ctx, params, req, res)

			if path := map[string]string{"runner": runner, "bundle": bundle}[tc.changed]; path != "" {
				if err := os.WriteFile(path, []byte("changed"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if got := New(dir, runner, bundle).Load(ctx, tc.params, tc.req); got != nil {
				t.Errorf("Load() = %v; want nil", got)
			}
		})
	}
}

func TestCacheExecutableRewritten(t *testing.T) {
	ctx := t.Context()
	runner, bundle, dir := setUp(t)
	New(dir, runner, bundle).Store(ctx, params, req, res)

	// A Cache must not be used after an executable was rewritten, since the
	// one pushed to the DUT may not be the hashed one.
	c := New(dir, runner, bundle)
	if err := os.WriteFile(bundle, []byte("rewritten"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := c.Load(ctx, params, req); got != nil {
		t.Errorf("Load() after rewriting an executable = %v; want nil", got)
	}
	c.Store(ctx, params, &protocol.ListEntitiesRequest{}, res)
	entries, err := filepath.Glob(filepath.Join(dir, "*"+entrySuffix))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("Cache has entries %v after rewriting an executable; want 1 entry", entries)
	}
}

func TestCacheBrokenEntry(t *testing.T) {
	ctx := t.Context()
	runner, bundle, dir := setUp(t)
	c := New(dir, runner, bundle)
	c.Store(ctx, params, req, res)

	path, err := c.path(params, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Entries may be written partially by a concurrent or interrupted run.
	flipped := slices.Clone(b)
	flipped[len(b)-1] ^= 1 // still a valid message
	sum := sha256.Sum256([]byte{0xff})
	noMessage := append(sum[:], 0xff) // a valid checksum of an invalid message
	for _, broken := range [][]byte{nil, b[:len(b)-1], flipped, noMessage} {
		if err := os.WriteFile(path, broken, 0644); err != nil {
			t.Fatal(err)
		}
		if got := c.Load(ctx, params, req); got != nil {
			t.Errorf("Load() for a broken entry of %d bytes = %v; want nil", len(broken), got)
		}
	}

	// Storing the response again repairs the entry.
	c.Store(ctx, params, req, res)
	if diff := cmp.Diff(c.Load(ctx, params, req), res, protocmp.Transform()); diff != "" {
		t.Errorf("Load() after storing again mismatch (-got +want):\n%s", diff)
	}
}

func TestCachePrune(t *testing.T) {
	ctx := t.Context()
	runner, bundle, dir := setUp(t)
	c := New(dir, runner, bundle)

	reqs := make([]*protocol.ListEntitiesRequest, maxEntries+2)
	for i := range reqs {
		reqs[i] = &protocol.ListEntitiesRequest{Features: &protocol.Features{
			Infra: &protocol.InfraFeatures{Vars: map[string]string{"i": fmt.Sprint(i)}},
		}}
		c.Store(ctx, params, reqs[i], res)
		// Make entries look as if they were stored a minute apart, since the
		// modification times may not differ otherwise.
		path, err := c.path(params, reqs[i])
		if err != nil {
			t.Fatal(err)
		}
		mtime := time.Now().Add(time.Duration(i-len(reqs)) * time.Minute)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	// Only the newest entries are kept.
	for i, r := range reqs {
		if got, want := c.Load(ctx, params, r) != nil, i >= len(reqs)-maxEntries; got != want {
			t.Errorf("Load(reqs[%d]) found an entry = %v; want %v", i, got, want)
		}
	}
}
