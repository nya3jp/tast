// Copyright 2018 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/testutil"
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

// checkDir checks that the dir at path exists and has the supplied mode.
// Test errors are reported for any discrepancies.
func checkDir(t *testing.T, path string, mode os.FileMode) {
	if di, err := os.Stat(path); err != nil {
		t.Errorf("Failed to stat %v: %v", path, err)
	} else if di.Mode()&os.ModeType != os.ModeDir {
		t.Errorf("%s is not a dir", path)
	} else if newMode := di.Mode() & os.ModePerm; newMode != mode {
		t.Errorf("%v has mode 0%o; want 0%o", path, newMode, mode)
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

func TestCopyDir(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Create the following dir/file structure:
	//   src mode:0755
	//   ├─file1 mode:0655 content:"file1"
	//   └─subdir mode:0754
	//     ├─file2 mode:0654 content:"file2"
	//     └─file3 <symlink to file1>
	src := filepath.Join(td, "src")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	file1Path := filepath.Join(src, "file1")
	if err := ioutil.WriteFile(file1Path, []byte("file1"), 0655); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "subdir"), 0754); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(src, "subdir", "file2"), []byte("file2"), 0654); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file1Path, filepath.Join(src, "subdir", "file3")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(td, "dst")

	if err := fsutil.CopyDir(src, dst); err != nil {
		t.Errorf("CopyDir unexpectedly failed: %v", err)
	}

	checkDir(t, filepath.Join(dst), 0755)
	checkFile(t, filepath.Join(dst, "file1"), "file1", 0655)
	checkDir(t, filepath.Join(dst, "subdir"), 0754)
	checkFile(t, filepath.Join(dst, "subdir", "file2"), "file2", 0654)
	checkFile(t, filepath.Join(dst, "subdir", "file3"), "file1", 0655)
}

func TestCopyDirFailsWhenDestinationExists(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	src := filepath.Join(td, "src")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(td, "dst")
	if err := os.Mkdir(dst, 0755); err != nil {
		t.Fatal(err)
	}

	if err := fsutil.CopyDir(src, dst); err == nil {
		t.Errorf("CopyDir unexpectedly succeeded: %v", err)
	}

	// Ensure destination directory is not removed.
	checkDir(t, filepath.Join(dst), 0755)
}

func TestCopyDirRemovesDestinationItCreatedOnFailure(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	src := filepath.Join(td, "src")
	// Reading directory with permissions 0100 will fail.
	if err := os.Mkdir(src, 0100); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(td, "dst")

	if err := fsutil.CopyDir(src, dst); err == nil {
		t.Errorf("CopyDir unexpectedly succeeded: %v", err)
	}

	if _, err := os.Stat(dst); err == nil {
		t.Errorf("Target directory %s has not been removed", dst)
	}
}
