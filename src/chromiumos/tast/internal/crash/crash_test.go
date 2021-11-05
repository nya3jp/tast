// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"chromiumos/tast/internal/crash"
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
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	writeCrashFile(t, td, "foo.txt", "") // skipped because non-core/dmp extension
	fooCore := writeCrashFile(t, td, "foo.core", "")
	fooDmp := writeCrashFile(t, td, "foo.dmp", "")
	fooLog := writeCrashFile(t, td, "foo.log", "")
	fooMeta := writeCrashFile(t, td, "foo.meta", "")
	fooGPU := writeCrashFile(t, td, "foo.i915_error_state.log.xz", "")
	fooCompressedTxt := writeCrashFile(t, td, "foo.txt.gz", "")
	fooBIOSLog := writeCrashFile(t, td, "foo.bios_log", "")
	fooKCrash := writeCrashFile(t, td, "foo.kcrash", "")
	fooCompressedLog := writeCrashFile(t, td, "foo.log.gz", "")
	barDmp := writeCrashFile(t, td, "bar.dmp", "")
	writeCrashFile(t, td, "bar", "")            // skipped because no extenison
	writeCrashFile(t, td, "subdir/baz.dmp", "") // skipped because in subdir
	writeCrashFile(t, td, "foo.info.gz", "")    // skipped because second extension is wrong
	writeCrashFile(t, td, "other.xz", "")

	dirs := []string{filepath.Join(td, "missing"), td} // nonexistent dir should be skipped
	files, err := crash.GetCrashes(dirs...)
	if err != nil {
		t.Fatalf("GetCrashes(%v) failed: %v", dirs, err)
	}
	sort.Strings(files)
	if exp := []string{barDmp.abs, fooBIOSLog.abs, fooCore.abs, fooDmp.abs, fooGPU.abs, fooKCrash.abs, fooLog.abs, fooCompressedLog.abs, fooMeta.abs, fooCompressedTxt.abs}; !reflect.DeepEqual(files, exp) {
		t.Errorf("GetCrashes(%v) = %v; want %v", dirs, files, exp)
	}
}

func TestCopyNewFiles(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	sd := filepath.Join(td, "src")
	a0 := writeCrashFile(t, sd, "a.dmp", "a0")
	a1 := writeCrashFile(t, sd, "a.meta", "a1")
	a2 := writeCrashFile(t, sd, "a.core", "a2")
	b0 := writeCrashFile(t, sd, "b.dmp", "b0")
	c0 := writeCrashFile(t, sd, "c.dmp", "c0")
	d0 := writeCrashFile(t, sd, "d.dmp", "d0")
	d1 := writeCrashFile(t, sd, "d.meta", "d1")
	d2 := writeCrashFile(t, sd, "d.core", "d2")

	dd := filepath.Join(td, "dst")
	if err := os.MkdirAll(dd, 0755); err != nil {
		t.Fatal(err)
	}
	op := []string{b0.abs}
	np := []string{a0.abs, a1.abs, a2.abs, b0.abs, c0.abs, d0.abs, d1.abs, d2.abs}
	if err := crash.CopyNewFiles(context.Background(), dd, np, op); err != nil {
		t.Fatalf("CopyNewFiles(%v, %v, %v) failed: %v", dd, np, op, err)
	}

	if fs, err := testutil.ReadFiles(dd); err != nil {
		t.Fatal(err)
	} else if exp := map[string]string{
		a0.rel: a0.data,
		a1.rel: a1.data,
		// a2 is skipped since it is a core dump.
		// b0 should be skipped since it already existed.
		c0.rel: c0.data,
		d0.rel: d0.data,
		d1.rel: d1.data,
		// d2 is skipped since it is a core dump.
	}; !reflect.DeepEqual(fs, exp) {
		t.Errorf("CopyNewFiles(%v, %v, %v) wrote %v; want %v", dd, np, op, fs, exp)
	}
}
