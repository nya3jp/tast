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

// getModulePaths returns just the sorted paths (i.e. keys) from m.
func getModulePaths(m SymbolFileMap) []string {
	ps := make([]string, 0)
	for p, _ := range m {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

func TestWriteSymbolFileAndWalkMinidump(t *testing.T) {
	td := testutil.TempDir(t, "minidump_test.")
	defer os.RemoveAll(td)

	const (
		// Relative path to checked-in minidump file used for testing.
		minidumpPath = "testdata/abort.20180103.145440.20827.dmp"

		// Relative path to checked-in executable with debugging symbols.
		abortDebugPath = "testdata/abort.debug"

		// On-device paths to modules referenced in minidumpPath.
		ldModulePath    = "/lib64/ld-2.23.so"
		libcModulePath  = "/lib64/libc-2.23.so"
		abortModulePath = "/usr/local/bin/abort"
	)

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
		{minidumpMagic[:len(minidumpMagic)-1], false},
		{minidumpMagic, true},
		{minidumpMagic + "blah", true},
	} {
		if valid, err := IsMinidump(bytes.NewBufferString(tc.data)); err != nil {
			t.Errorf("IsMinidump(%q) failed: %v", tc.data, err)
		} else if valid != tc.valid {
			t.Errorf("IsMinidump(%q) = %v; want %v", tc.data, valid, tc.valid)
		}
	}
}
