// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fsutil_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"chromiumos/tast/fsutil"
	"chromiumos/tast/testutil"
)

// setUpFile creates a temporary directory containing a file with the supplied data and mode.
// The temporary directory's and file's paths are returned.
// A fatal test error is reported if any operations fail.
func setUpFile(t *testing.T, data string, mode os.FileMode) (td, fn string) {
	td = testutil.TempDir(t)
	fn = filepath.Join(td, "src.txt")
	if err := ioutil.WriteFile(fn, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fn, mode); err != nil {
		t.Fatal(err)
	}
	return td, fn
}

// checkFile checks that the file at path has the supplied data and mode.
// Test errors are reported for any discrepancies.
func checkFile(t *testing.T, path, data string, mode os.FileMode) {
	if fi, err := os.Stat(path); err != nil {
		t.Errorf("Failed to stat %v: %v", path, err)
	} else if newMode := fi.Mode() & os.ModePerm; newMode != mode {
		t.Errorf("%v has mode 0%o; want 0%o", path, newMode, mode)
	}

	if b, err := ioutil.ReadFile(path); err != nil {
		t.Errorf("Failed to read %v: %v", path, err)
	} else if string(b) != data {
		t.Errorf("%v contains %q; want %q", path, string(b), data)
	}
}

func TestCopyFile(t *testing.T) {
	const (
		data = "this is not the most interesting text ever written"
		mode = 0461
	)
	td, src := setUpFile(t, data, mode)
	defer os.RemoveAll(td)

	dst := filepath.Join(td, "dst.txt")
	if err := fsutil.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile(%q, %q) failed: %v", src, dst, err)
	}

	checkFile(t, dst, data, mode)
}

func TestMoveFile(t *testing.T) {
	const (
		data = "another boring file"
		mode = 0401
	)
	td, src := setUpFile(t, data, mode)
	defer os.RemoveAll(td)

	dst := filepath.Join(td, "dst.txt")
	if err := fsutil.MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile(%q, %q) failed: %v", src, dst, err)
	}

	checkFile(t, dst, data, mode)

	if _, err := os.Stat(src); err == nil {
		t.Errorf("%s still exists", src)
	} else if !os.IsNotExist(err) {
		t.Errorf("Failed to stat %s: %v", src, err)
	}
}

func TestMoveFileAcrossFilesystems(t *testing.T) {
	const (
		data = "another boring file"
		mode = 0401
	)
	td, src := setUpFile(t, data, mode)
	defer os.RemoveAll(td)

	var srcStat unix.Statfs_t
	if err := unix.Statfs(src, &srcStat); err != nil {
		t.Fatal(err)
	}

	// Check that we're actually crossing filesystems.
	const dstFS = "/dev/shm"
	var dstStat unix.Statfs_t
	if err := unix.Statfs(dstFS, &dstStat); err != nil {
		t.Logf("Failed to stat %v; skipping test: %v", dstFS, err)
		return
	} else if srcStat.Fsid == dstStat.Fsid {
		t.Logf("FS for %v and %v both have ID %v; skipping test", src, srcStat.Fsid, dstStat.Fsid)
		return
	}
	df, err := ioutil.TempFile(dstFS, "dst.txt.")
	if err != nil {
		t.Fatal(err)
	}
	df.Close()
	dst := df.Name()
	defer os.Remove(dst)

	if err := fsutil.MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile(%q, %q) failed: %v", src, dst, err)
	}

	checkFile(t, dst, data, mode)

	if _, err := os.Stat(src); err == nil {
		t.Errorf("%s still exists", src)
	} else if !os.IsNotExist(err) {
		t.Errorf("Failed to stat %s: %v", src, err)
	}
}

func TestCopyFileOrMoveFileWithDir(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	src := filepath.Join(td, "dir")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}

	// Both functions should reject directories.
	if err := fsutil.CopyFile(src, filepath.Join(td, "copyDst")); err == nil {
		t.Error("CopyFile unexpectedly succeeded for directory ", src)
	}
	if err := fsutil.MoveFile(src, filepath.Join(td, "moveDst")); err == nil {
		t.Error("MoveFile unexpectedly succeeded for directory ", src)
	}
}
