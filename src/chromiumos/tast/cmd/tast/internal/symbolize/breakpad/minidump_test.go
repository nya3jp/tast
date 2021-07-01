// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"chromiumos/tast/testutil"
)

const (
	// Relative path to checked-in minidump file used for testing.
	minidumpPath = "testdata/abort.20180103.145440.12345.20827.dmp"

	// Minidump with Crashpad annotations.
	minidumpCrashpadPath = "testdata/chrome.20210706.000145.15087.8090.dmp"

	// Relative path to checked-in executable with debugging symbols.
	abortDebugPath = "testdata/abort.debug"

	// On-device paths to modules referenced in minidumpPath.
	ldModulePath    = "/lib64/ld-2.23.so"
	libcModulePath  = "/lib64/libc-2.23.so"
	abortModulePath = "/usr/local/bin/abort"
)

// getModulePaths returns just the sorted paths (i.e. keys) from m.
func getModulePaths(m SymbolFileMap) []string {
	ps := make([]string, 0)
	for p := range m {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

func TestWriteSymbolFileAndWalkMinidump(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// When we first walk the minidump file's stack, symbols should be missing.
	b := bytes.Buffer{}
	missing, err := WalkMinidump(minidumpPath, td, &b)
	if err != nil {
		t.Fatalf("WalkMinidump(%v, %v, ...) failed: %v", minidumpPath, td, err)
	}
	if act, exp := getModulePaths(missing), []string{ldModulePath, libcModulePath, abortModulePath}; !reflect.DeepEqual(act, exp) {
		t.Errorf("WalkMinidump(%v, %v, ...) returned missing files %v; want %v",
			minidumpPath, td, act, exp)
	}
	// We shouldn't be able to see the filename yet.
	if str := "abort.c"; strings.Contains(b.String(), str) {
		t.Errorf("WalkMinidump(%v, %v, ...)'s output already includes %q (bad testdata?); full:\n%v",
			minidumpPath, td, str, b.String())
	}

	// Write a symbol file for the executable and check that the expected module record is generated.
	mi, err := WriteSymbolFile(abortDebugPath, td)
	if err != nil {
		t.Fatalf("WriteSymbolFile(%v, %v) failed: %v", abortDebugPath, td, err)
	}
	if exp := filepath.Base(abortDebugPath); mi.Name != exp {
		t.Errorf("WriteSymbolFile(%v, %v) returned module path %q; want %q",
			abortDebugPath, td, mi.Name, exp)
	}
	if mi.ID != missing[abortModulePath] {
		t.Errorf("WriteSymbolFile(%v, %v) returned module ID %q; want %q",
			abortDebugPath, td, mi.ID, missing[abortModulePath])
	}

	// When we walk the stack again, only libc and ld's symbols should be missing.
	b.Reset()
	if missing, err = WalkMinidump(minidumpPath, td, &b); err != nil {
		t.Fatalf("WalkMinidump(%v, %v, ...) failed: %v", minidumpPath, td, err)
	}
	if act, exp := getModulePaths(missing), []string{ldModulePath, libcModulePath}; !reflect.DeepEqual(act, exp) {
		t.Errorf("WalkMinidump(%v, %v, ...) returned missing files %v; want %v",
			minidumpPath, td, act, exp)
	}
	// Check that the stack trace contains symbols from the executable. Here's the full frame:
	//   1  abort!main [abort.c : 4 + 0x5]
	//	    rbp = 0x00007fff46c2b2d0   rsp = 0x00007fff46c2b2c0
	//	    rip = 0x0000000000400541
	//	    Found by: previous frame's frame pointer
	if str := "abort!main [abort.c : 4"; !strings.Contains(b.String(), str) {
		t.Errorf("WalkMinidump(%v, %v, ...)'s output didn't contain %q; full:\n%v",
			minidumpPath, td, str, b.String())
	}
}

func TestIsMinidump(t *testing.T) {
	for _, tc := range []struct {
		data  string
		valid bool
	}{
		{"", false},
		{"DATA", false},
		{mdMagic[:len(mdMagic)-1], false},
		{mdMagic, true},
		{mdMagic + "blah", true},
	} {
		if valid, err := IsMinidump(bytes.NewBufferString(tc.data)); err != nil {
			t.Errorf("IsMinidump(%q) failed: %v", tc.data, err)
		} else if valid != tc.valid {
			t.Errorf("IsMinidump(%q) = %v; want %v", tc.data, valid, tc.valid)
		}
	}
}

func TestGetMinidumpReleaseInfo(t *testing.T) {
	tests := []struct {
		desc       string
		path       string
		verifyFunc func(*testing.T, *MinidumpReleaseInfo)
	}{
		{
			desc: "with_etc_lsb-release",
			path: minidumpPath,
			verifyFunc: func(t *testing.T, ri *MinidumpReleaseInfo) {
				// Just check that the returned data starts and ends correctly.
				if !strings.HasPrefix(ri.EtcLsbRelease, "CHROMEOS_RELEASE_APPID=") ||
					!strings.HasSuffix(ri.EtcLsbRelease, ".google.com:8080/update\n") {
					t.Errorf("GetMinidumpReleaseInfo returned unexpected result %q", ri)
				}
			},
		},
		{
			desc: "with_crashpad_annotations",
			path: minidumpCrashpadPath,
			verifyFunc: func(t *testing.T, ri *MinidumpReleaseInfo) {
				expectedBoard := "betty"
				if board := ri.CrashpadAnnotations["chromeos-board"]; board != expectedBoard {
					t.Errorf("Expected chromeos-board = %q, got: %q", expectedBoard, board)
				}
				expectedBuilderPath := "betty-release/R93-14070.0.0"
				if builderPath := ri.CrashpadAnnotations["chromeos-builder-path"]; builderPath != expectedBuilderPath {
					t.Errorf("Expected chromeos-builder-path = %q, got: %q", expectedBuilderPath, builderPath)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			f, err := os.Open(test.path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			ri, err := GetMinidumpReleaseInfo(f)
			if err != nil {
				t.Fatal(err)
			}

			test.verifyFunc(t, ri)
		})
	}
}
