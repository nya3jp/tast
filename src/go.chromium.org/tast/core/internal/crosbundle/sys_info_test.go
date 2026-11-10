// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/testutil"
)

func TestCollectCrashFiles(t *testing.T) {
	srcDir := t.TempDir()
	if err := testutil.WriteFiles(srcDir, map[string]string{
		"old.dmp":  "old dump",
		"new.dmp":  "new dump",
		"big.core": "core dump",
	}); err != nil {
		t.Fatalf("testutil.WriteFiles(%q) failed: %v", srcDir, err)
	}
	oldDump := filepath.Join(srcDir, "old.dmp")
	newDump := filepath.Join(srcDir, "new.dmp")
	coreDump := filepath.Join(srcDir, "big.core")

	for _, tc := range []struct {
		name      string
		dumps     []string
		initial   []string
		wantFiles map[string]string // nil means expect "" and no temp dir left behind
	}{
		{
			name: "no_dumps",
		},
		{
			name:    "all_already_present",
			dumps:   []string{oldDump},
			initial: []string{oldDump},
		},
		{
			name:  "only_core_files",
			dumps: []string{coreDump},
		},
		{
			name:      "new_crash_file",
			dumps:     []string{oldDump, newDump, coreDump},
			initial:   []string{oldDump},
			wantFiles: map[string]string{"new.dmp": "new dump"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("TMPDIR", tmpDir)

			crashDir, err := collectCrashFiles(t.Context(), tc.dumps, tc.initial)
			if err != nil {
				t.Fatalf("collectCrashFiles(%v, %v) failed: %v", tc.dumps, tc.initial, err)
			}

			if tc.wantFiles == nil {
				if crashDir != "" {
					t.Errorf("collectCrashFiles(%v, %v) = %q; want %q", tc.dumps, tc.initial, crashDir, "")
				}
				entries, err := os.ReadDir(tmpDir)
				if err != nil {
					t.Fatalf("os.ReadDir(%q) failed: %v", tmpDir, err)
				}
				if len(entries) != 0 {
					t.Errorf("collectCrashFiles(%v, %v) left %d temp entries in %q; want 0", tc.dumps, tc.initial, len(entries), tmpDir)
				}
				return
			}

			if crashDir == "" {
				t.Fatalf("collectCrashFiles(%v, %v) = %q; want non-empty dir", tc.dumps, tc.initial, crashDir)
			}
			if filepath.Dir(crashDir) != tmpDir {
				t.Errorf("collectCrashFiles(%v, %v) = %q; want dir under %q", tc.dumps, tc.initial, crashDir, tmpDir)
			}
			gotFiles, err := testutil.ReadFiles(crashDir)
			if err != nil {
				t.Fatalf("testutil.ReadFiles(%q) failed: %v", crashDir, err)
			}
			if diff := cmp.Diff(gotFiles, tc.wantFiles); diff != "" {
				t.Errorf("collectCrashFiles(%v, %v) files mismatch (-got +want):\n%s", tc.dumps, tc.initial, diff)
			}
		})
	}
}
