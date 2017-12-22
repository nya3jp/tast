// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"chromiumos/tast/testutil"
)

// crashFile contains information about a crash file used by tests.
// The testutil package uses relative paths while the crash package
// uses absolute paths, so this struct stores both.
type crashFile struct{ rel, abs, data string }

// writeCrashFile writes a file with relative path rel containing data to dir.
func writeCrashFile(t *testing.T, dir, rel, data string) crashFile {
	cf := crashFile{rel, filepath.Join(dir, rel), data}
	if err := testutil.WriteFiles(dir, map[string]string{rel: data}); err != nil {
		t.Fatal(err)
	}
	return cf
}

func TestGetCrashes(t *testing.T) {
	td := testutil.TempDir(t, "crash_test.")
	defer os.RemoveAll(td)

	writeCrashFile(t, td, "foo.txt", "") // skipped because non-core/dmp extension
	fooCore := writeCrashFile(t, td, "foo.core", "")
	fooDmp := writeCrashFile(t, td, "foo.dmp", "")
	barDmp := writeCrashFile(t, td, "bar.dmp", "")
	writeCrashFile(t, td, "bar", "")            // skipped because no extenison
	writeCrashFile(t, td, "subdir/baz.dmp", "") // skipped because in subdir

	cs, mds, err := GetCrashes(td)
	if err != nil {
		t.Fatalf("GetCrashes(%v) failed: %v", td, err)
	}
	sort.Strings(cs)
	if exp := []string{fooCore.abs}; !reflect.DeepEqual(cs, exp) {
		t.Errorf("GetCrashes(%v) = %v; want %v", td, cs, exp)
	}
	sort.Strings(mds)
	if exp := []string{barDmp.abs, fooDmp.abs}; !reflect.DeepEqual(mds, exp) {
		t.Errorf("GetCrashes(%v) = %v; want %v", td, mds, exp)
	}
}

func TestCopyNewFiles(t *testing.T) {
	td := testutil.TempDir(t, "crash_test.")
	defer os.RemoveAll(td)

	sd := filepath.Join(td, "src")
	a0 := writeCrashFile(t, sd, "a.0.dmp", "a0")
	a1 := writeCrashFile(t, sd, "a.1.dmp", "a1")
	a2 := writeCrashFile(t, sd, "a.2.dmp", "a2")
	b0 := writeCrashFile(t, sd, "b.0.dmp", "b0")
	c0 := writeCrashFile(t, sd, "c.0.dmp", "c0")
	// Chrome writes files without a Chrome prefix, e.g. "dcafa20c-4eca-47a7-136b0080-44a9fc7f.dmp".
	d0 := writeCrashFile(t, sd, "d0.dmp", "d0")
	d1 := writeCrashFile(t, sd, "d1.dmp", "d1")
	d2 := writeCrashFile(t, sd, "d2.dmp", "d2")

	dd := filepath.Join(td, "dst")
	if err := os.MkdirAll(dd, 0755); err != nil {
		t.Fatal(err)
	}
	op := []string{b0.abs}
	np := []string{a0.abs, a1.abs, a2.abs, b0.abs, c0.abs, d0.abs, d1.abs, d2.abs}
	max := 2
	if _, err := CopyNewFiles(dd, np, op, max); err != nil {
		t.Fatalf("CopyNewFiles(%v, %v, %v, %v) failed: %v", dd, np, op, max, err)
	}

	if fs, err := testutil.ReadFiles(dd); err != nil {
		t.Fatal(err)
	} else if exp := map[string]string{
		a0.rel: a0.data,
		a1.rel: a1.data,
		// a2 should be skipped since we've already seen two files for "a".
		// b0 should be skipped since it already existed.
		c0.rel: c0.data,
		d0.rel: d0.data,
		d1.rel: d1.data,
		// d2 should be skipped since we've already seen two non-prefixed files.
	}; !reflect.DeepEqual(fs, exp) {
		t.Errorf("CopyNewFiles(%v, %v, %v, %v) wrote %v; want %v", dd, np, op,
			max, fs, exp)
	}
}
