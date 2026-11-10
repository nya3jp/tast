// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/protocol"
)

func TestComputeDUTInfoCacheKey(t *testing.T) {
	td := t.TempDir()
	useFlags := filepath.Join(td, "use_flags.txt")
	if err := os.WriteFile(useFlags, []byte("flag1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(td, "missing")
	capDir := filepath.Join(td, "caps")
	if err := os.Mkdir(capDir, 0755); err != nil {
		t.Fatal(err)
	}
	capFile := filepath.Join(capDir, "10-cap.yaml")
	if err := os.WriteFile(capFile, []byte("- cap1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	paths := []string{useFlags, missing, capDir}
	req := &protocol.GetDUTInfoRequest{Features: true, AllowCache: true, ExtraUseFlags: []string{"a"}}

	orig, err := computeDUTInfoCacheKey(req, "boot1\n", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey(req, %q, paths) failed: %v", "boot1\n", err)
	}

	// Trailing whitespace on boot_id (from /proc/sys/kernel/random/boot_id) is
	// trimmed.
	if k, err := computeDUTInfoCacheKey(req, "boot1", paths); err != nil {
		t.Fatalf("computeDUTInfoCacheKey(req, %q, paths) failed: %v", "boot1", err)
	} else if k != orig {
		t.Errorf("computeDUTInfoCacheKey(req, %q, paths) = %q, want %q (trimmed bootID)", "boot1", k, orig)
	}

	if k, err := computeDUTInfoCacheKey(req, "boot2", paths); err != nil {
		t.Fatalf("computeDUTInfoCacheKey(req, %q, paths) failed: %v", "boot2", err)
	} else if k == orig {
		t.Errorf("computeDUTInfoCacheKey(req, %q, paths) = %q, want different key from %q", "boot2", k, orig)
	}

	otherReq := &protocol.GetDUTInfoRequest{Features: true, AllowCache: true, ExtraUseFlags: []string{"b"}}
	if k, err := computeDUTInfoCacheKey(otherReq, "boot1", paths); err != nil {
		t.Fatalf("computeDUTInfoCacheKey(otherReq, %q, paths) failed: %v", "boot1", err)
	} else if k == orig {
		t.Errorf("computeDUTInfoCacheKey(otherReq, %q, paths) = %q, want different key from %q", "boot1", k, orig)
	}

	// File size change invalidates the key.
	if err := os.WriteFile(useFlags, []byte("flag1\nflag2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	afterSizeChange, err := computeDUTInfoCacheKey(req, "boot1", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey after size change failed: %v", err)
	}
	if afterSizeChange == orig {
		t.Errorf("computeDUTInfoCacheKey after size change = %q, want different key from %q", afterSizeChange, orig)
	}

	// Advancing mtime without changing size invalidates the key.
	fi, err := os.Stat(useFlags)
	if err != nil {
		t.Fatal(err)
	}
	newMtime := fi.ModTime().Add(time.Minute)
	if err := os.Chtimes(useFlags, newMtime, newMtime); err != nil {
		t.Fatal(err)
	}
	afterMtimeChange, err := computeDUTInfoCacheKey(req, "boot1", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey after mtime change failed: %v", err)
	}
	if afterMtimeChange == afterSizeChange {
		t.Errorf("computeDUTInfoCacheKey after mtime change = %q, want different key from %q", afterMtimeChange, afterSizeChange)
	}

	// File creation invalidates the key.
	if err := os.WriteFile(missing, nil, 0644); err != nil {
		t.Fatal(err)
	}
	afterCreate, err := computeDUTInfoCacheKey(req, "boot1", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey after file creation failed: %v", err)
	}
	if afterCreate == afterMtimeChange {
		t.Errorf("computeDUTInfoCacheKey after file creation = %q, want different key from %q", afterCreate, afterMtimeChange)
	}

	// File removal invalidates the key.
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	afterRemove, err := computeDUTInfoCacheKey(req, "boot1", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey after file removal failed: %v", err)
	}
	if afterRemove == afterCreate {
		t.Errorf("computeDUTInfoCacheKey after file removal = %q, want different key from %q", afterRemove, afterCreate)
	}

	// In-place edit to an existing file inside a watched directory invalidates
	// the key even though the directory's own mtime does not change.
	if err := os.WriteFile(capFile, []byte("- cap2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	capFI, err := os.Stat(capFile)
	if err != nil {
		t.Fatal(err)
	}
	newCapMtime := capFI.ModTime().Add(time.Minute)
	if err := os.Chtimes(capFile, newCapMtime, newCapMtime); err != nil {
		t.Fatal(err)
	}
	afterDirEdit, err := computeDUTInfoCacheKey(req, "boot1", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey after directory entry edit failed: %v", err)
	}
	if afterDirEdit == afterRemove {
		t.Errorf("computeDUTInfoCacheKey after directory entry edit = %q, want different key from %q", afterDirEdit, afterRemove)
	}

	// Adding a file inside a watched directory invalidates the key.
	if err := os.WriteFile(filepath.Join(capDir, "20-extra.yaml"), []byte("- cap3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	afterDirAdd, err := computeDUTInfoCacheKey(req, "boot1", paths)
	if err != nil {
		t.Fatalf("computeDUTInfoCacheKey after directory entry addition failed: %v", err)
	}
	if afterDirAdd == afterDirEdit {
		t.Errorf("computeDUTInfoCacheKey after directory entry addition = %q, want different key from %q", afterDirAdd, afterDirEdit)
	}

	// Non-ErrNotExist stat error (ENOTDIR via file/child) returns an error so
	// callers fall back to uncached detection rather than hashing "missing".
	badPath := filepath.Join(useFlags, "child")
	if _, err := computeDUTInfoCacheKey(req, "boot1", []string{badPath}); err == nil {
		t.Errorf("computeDUTInfoCacheKey(req, %q, [%q]) error = nil, want non-nil", "boot1", badPath)
	}
}

func TestDUTInfoCacheGet(t *testing.T) {
	ctx := t.Context()
	cachePath := filepath.Join(t.TempDir(), "tast", "dut_info_cache.pb")
	req := &protocol.GetDUTInfoRequest{Features: true, AllowCache: true}

	key := "key1"
	calls := 0
	c := dutInfoCache{
		path:  cachePath,
		keyFn: func(*protocol.GetDUTInfoRequest) (string, error) { return key, nil },
		getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			calls++
			return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: fmt.Sprintf("v%d", calls)}}, nil
		},
	}

	steps := []struct {
		desc        string
		key         string
		wantCalls   int
		wantVersion string
	}{
		{desc: "empty cache", key: "key1", wantCalls: 1, wantVersion: "v1"},
		{desc: "same key", key: "key1", wantCalls: 1, wantVersion: "v1"},
		{desc: "different key", key: "key2", wantCalls: 2, wantVersion: "v2"},
	}
	for _, s := range steps {
		key = s.key
		res, err := c.get(ctx, req)
		if err != nil {
			t.Fatalf("%s: dutInfoCache.get() failed: %v", s.desc, err)
		}
		if calls != s.wantCalls {
			t.Errorf("%s: getFn called %d times, want %d", s.desc, calls, s.wantCalls)
		}
		want := &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: s.wantVersion}}
		if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
			t.Errorf("%s: dutInfoCache.get() returned unexpected diff (-want +got):\n%s", s.desc, diff)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(cachePath))
	if err != nil {
		t.Fatalf("os.ReadDir(%q) failed: %v", filepath.Dir(cachePath), err)
	}
	var gotNames []string
	for _, e := range entries {
		gotNames = append(gotNames, e.Name())
	}
	wantNames := []string{filepath.Base(cachePath)}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Errorf("cache directory entries diff (-want +got):\n%s", diff)
	}
}

func TestDUTInfoCacheGetBypass(t *testing.T) {
	cases := []struct {
		name        string
		req         *protocol.GetDUTInfoRequest
		wantVersion string
	}{
		{
			name:        "allow_cache_false",
			req:         &protocol.GetDUTInfoRequest{Features: true, AllowCache: false},
			wantVersion: "fresh",
		},
		{
			name:        "features_false",
			req:         &protocol.GetDUTInfoRequest{Features: false, AllowCache: true},
			wantVersion: "no-features",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cachePath := filepath.Join(t.TempDir(), "dut_info_cache.pb")
			cachedRes := &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "cached"}}
			if err := saveDUTInfoCache(cachePath, "key1", cachedRes); err != nil {
				t.Fatalf("saveDUTInfoCache() failed: %v", err)
			}
			origCache, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}

			calls := 0
			c := dutInfoCache{
				path: cachePath,
				keyFn: func(*protocol.GetDUTInfoRequest) (string, error) {
					t.Errorf("keyFn was called for %s", tc.name)
					return "key1", nil
				},
				getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
					calls++
					return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: tc.wantVersion}}, nil
				},
			}
			res, err := c.get(t.Context(), tc.req)
			if err != nil {
				t.Fatalf("dutInfoCache.get() failed: %v", err)
			}
			if calls != 1 {
				t.Errorf("getFn called %d times, want 1", calls)
			}
			want := &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: tc.wantVersion}}
			if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
				t.Errorf("dutInfoCache.get() returned unexpected diff (-want +got):\n%s", diff)
			}
			afterCache, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(origCache, afterCache); diff != "" {
				t.Errorf("cache file modified during %s (-want +got):\n%s", tc.name, diff)
			}
		})
	}
}

func TestDUTInfoCacheGetKeyError(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "dut_info_cache.pb")
	calls := 0
	c := dutInfoCache{
		path: cachePath,
		keyFn: func(*protocol.GetDUTInfoRequest) (string, error) {
			return "", errors.New("boot_id unavailable")
		},
		getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			calls++
			return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "fallback"}}, nil
		},
	}
	res, err := c.get(t.Context(), &protocol.GetDUTInfoRequest{Features: true, AllowCache: true})
	if err != nil {
		t.Fatalf("dutInfoCache.get() failed: %v", err)
	}
	if calls != 1 {
		t.Errorf("getFn called %d times, want 1", calls)
	}
	want := &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "fallback"}}
	if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
		t.Errorf("dutInfoCache.get() returned unexpected diff (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) error = %v, want %v", cachePath, err, os.ErrNotExist)
	}
}

func TestDUTInfoCacheGetCorrupt(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
	}{
		{name: "invalid_proto", content: []byte("key1\nbroken")},
		{name: "missing_newline", content: []byte("key1")},
		{name: "empty_file", content: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			cachePath := filepath.Join(t.TempDir(), "dut_info_cache.pb")
			if err := os.WriteFile(cachePath, tc.content, 0644); err != nil {
				t.Fatal(err)
			}
			req := &protocol.GetDUTInfoRequest{Features: true, AllowCache: true}
			calls := 0
			c := dutInfoCache{
				path:  cachePath,
				keyFn: func(*protocol.GetDUTInfoRequest) (string, error) { return "key1", nil },
				getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
					calls++
					return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: fmt.Sprintf("v%d", calls)}}, nil
				},
			}

			want := &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "v1"}}
			res, err := c.get(ctx, req)
			if err != nil {
				t.Fatalf("dutInfoCache.get() on corrupt cache failed: %v", err)
			}
			if calls != 1 {
				t.Errorf("getFn called %d times on corrupt cache, want 1", calls)
			}
			if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
				t.Errorf("dutInfoCache.get() on corrupt cache diff (-want +got):\n%s", diff)
			}

			res, err = c.get(ctx, req)
			if err != nil {
				t.Fatalf("dutInfoCache.get() on repaired cache failed: %v", err)
			}
			if calls != 1 {
				t.Errorf("getFn called %d times after repair, want 1", calls)
			}
			if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
				t.Errorf("dutInfoCache.get() on repaired cache diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDUTInfoCacheGetSaveError(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "dut_info_cache.pb")
	// Making cachePath an existing directory causes os.Rename in saveDUTInfoCache
	// to fail reliably even when tests run as root.
	if err := os.Mkdir(cachePath, 0755); err != nil {
		t.Fatal(err)
	}

	c := dutInfoCache{
		path:  cachePath,
		keyFn: func(*protocol.GetDUTInfoRequest) (string, error) { return "key1", nil },
		getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "v1"}}, nil
		},
	}
	res, err := c.get(t.Context(), &protocol.GetDUTInfoRequest{Features: true, AllowCache: true})
	if err != nil {
		t.Fatalf("dutInfoCache.get() failed on save error: %v", err)
	}
	want := &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "v1"}}
	if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
		t.Errorf("dutInfoCache.get() returned unexpected diff (-want +got):\n%s", diff)
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) failed: %v", cacheDir, err)
	}
	var gotNames []string
	for _, e := range entries {
		gotNames = append(gotNames, e.Name())
	}
	wantNames := []string{filepath.Base(cachePath)}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Errorf("cache directory entries after save failure diff (-want +got):\n%s", diff)
	}
}

func TestDUTInfoCacheGetCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cachePath := filepath.Join(t.TempDir(), "dut_info_cache.pb")
	c := dutInfoCache{
		path:  cachePath,
		keyFn: func(*protocol.GetDUTInfoRequest) (string, error) { return "key1", nil },
		getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			cancel()
			return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{OsVersion: "partial"}}, nil
		},
	}

	if _, err := c.get(ctx, &protocol.GetDUTInfoRequest{Features: true, AllowCache: true}); !errors.Is(err, context.Canceled) {
		t.Errorf("dutInfoCache.get() error = %v, want %v", err, context.Canceled)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) error = %v, want %v", cachePath, err, os.ErrNotExist)
	}
}

func TestDUTInfoCacheGetError(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "dut_info_cache.pb")
	wantErr := errors.New("detection failed")
	c := dutInfoCache{
		path:  cachePath,
		keyFn: func(*protocol.GetDUTInfoRequest) (string, error) { return "key1", nil },
		getFn: func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return nil, wantErr
		},
	}

	if _, err := c.get(t.Context(), &protocol.GetDUTInfoRequest{Features: true, AllowCache: true}); !errors.Is(err, wantErr) {
		t.Errorf("dutInfoCache.get() error = %v, want %v", err, wantErr)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) error = %v, want %v", cachePath, err, os.ErrNotExist)
	}
}
