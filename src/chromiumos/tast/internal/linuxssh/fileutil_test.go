// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxssh_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/testutil"
)

// initFileTest creates a temporary directory with a subdirectory containing files.
// The temp dir's and subdir's paths are returned.
func initFileTest(t *testing.T, files map[string]string) (tmpDir, srcDir string) {
	tmpDir = testutil.TempDir(t)
	srcDir = filepath.Join(tmpDir, "src")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(srcDir, files); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	return tmpDir, srcDir
}

// checkFile returns an error if p's contents differ from exp.
func checkFile(p, exp string) error {
	if b, err := ioutil.ReadFile(p); err != nil {
		return fmt.Errorf("failed to read %v after copy: %v", p, err)
	} else if string(b) != exp {
		return fmt.Errorf("expected content %q after copy, got %v", exp, string(b))
	}
	return nil
}

// checkDir returns an error if dir's contents don't match exp, a map from paths to data.
func checkDir(dir string, exp map[string]string) error {
	if act, err := testutil.ReadFiles(dir); err != nil {
		return err
	} else if !reflect.DeepEqual(exp, act) {
		return fmt.Errorf("files not copied correctly: src %v, dst %v", exp, act)
	}
	return nil
}

func TestGetFileRegular(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "foo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local files.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
		t.Error(err)
	}

	// Using DereferenceSymlinks should make no difference for regular files
	srcFile = filepath.Join(srcDir, "file")
	dstFile = filepath.Join(tmpDir, "file.dereference")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.DereferenceSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local files.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.DereferenceSymlinks); err != nil {
		t.Error(err)
	}
}

func TestGetFileRegularSymlink(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "foo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	if err := os.Symlink("file", filepath.Join(srcDir, "link")); err != nil {
		t.Fatal(err)
	}

	srcFile := filepath.Join(srcDir, "link")
	dstFile := filepath.Join(tmpDir, "link.preserve")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	if linkname, err := os.Readlink(dstFile); err != nil {
		t.Error(err)
	} else if linkname != "file" {
		t.Error("Expected symlink to file, got ", linkname)
	}

	// GetFile should overwrite local files.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
		t.Error(err)
	}

	srcFile = filepath.Join(srcDir, "link")
	dstFile = filepath.Join(tmpDir, "link.dereference")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.DereferenceSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}
	if fi, err := os.Lstat(dstFile); err != nil {
		t.Error(err)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("Expected regular file, found symlink")
	}

	// GetFile should overwrite local files.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.DereferenceSymlinks); err != nil {
		t.Error(err)
	}
}

func TestGetFileDir(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{
		"myfile":     "some data",
		"mydir/file": "this is in a subdirectory",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)
	if err := os.Symlink("myfile", filepath.Join(srcDir, "filelink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("mydir", filepath.Join(srcDir, "dirlink")); err != nil {
		t.Fatal(err)
	}
	topDirLink := filepath.Join(filepath.Dir(srcDir), "topdirlink")
	if err := os.Symlink(filepath.Base(srcDir), topDirLink); err != nil {
		t.Fatal(err)
	}

	// Copy the full source directory.
	dstDir := filepath.Join(tmpDir, "dst")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcDir, dstDir, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}
	if linkname, err := os.Readlink(filepath.Join(dstDir, "filelink")); err != nil {
		t.Error(err)
	} else if linkname != "myfile" {
		t.Error("Expected symlink to myfile, got ", linkname)
	}
	if linkname, err := os.Readlink(filepath.Join(dstDir, "dirlink")); err != nil {
		t.Error(err)
	} else if linkname != "mydir" {
		t.Error("Expected symlink to mydir, got ", linkname)
	}
	// GetFile should overwrite local dirs.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcDir, dstDir, linuxssh.PreserveSymlinks); err != nil {
		t.Error(err)
	}

	// Copy the link to the full source directory.
	dstDir = filepath.Join(tmpDir, "dst.toplink")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, topDirLink, dstDir, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	if linkname, err := os.Readlink(dstDir); err != nil {
		t.Error(err)
	} else if linkname != filepath.Base(srcDir) {
		t.Errorf("Expected symlink to %s, got %s", filepath.Base(srcDir), linkname)
	}
	// GetFile should overwrite local dirs.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, topDirLink, dstDir, linuxssh.PreserveSymlinks); err != nil {
		t.Error(err)
	}

	files["filelink"] = files["myfile"]
	files["dirlink/file"] = files["mydir/file"]

	// Copy the full source directory dereferencing symlinks
	dstDir = filepath.Join(tmpDir, "dst.deference")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcDir, dstDir, linuxssh.DereferenceSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}
	// GetFile should overwrite local dirs.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcDir, dstDir, linuxssh.DereferenceSymlinks); err != nil {
		t.Error(err)
	}

	// Copy the full source directory dereferencing symlinks
	dstDir = filepath.Join(tmpDir, "dst.toplink.deference")
	if err := linuxssh.GetFile(td.Ctx, td.Hst, topDirLink, dstDir, linuxssh.DereferenceSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}
	// GetFile should overwrite local dirs.
	if err := linuxssh.GetFile(td.Ctx, td.Hst, topDirLink, dstDir, linuxssh.DereferenceSymlinks); err != nil {
		t.Error(err)
	}
}

func TestGetFileTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file")
	td.ExecTimeout = sshtest.StartTimeout
	if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err == nil {
		t.Errorf("GetFile() with expired context didn't return error")
	}
}

func TestPutFiles(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	const (
		weirdSrcName = "weird[a], *.b" // various regexp chars plus comma delimiter
		weirdDstName = "\\1\\a,&"      // sed replacement chars plus comma delimiter
	)
	files := map[string]string{
		"file1":             "file1 new",
		"dir/file2":         "file2 new",
		"dir2/subdir/file3": "file3 new",
		"file4":             "file4 new",
		weirdSrcName:        "file5 new",
		"file6":             "file6 new",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if err := testutil.WriteFiles(dstDir, map[string]string{
		"file1":             "file1 old",
		"dir/file2":         "file2 old",
		"dir2/subdir/file3": "file3 old",
		"file4":             "file4 old",
	}); err != nil {
		t.Fatal(err)
	}

	if n, err := linuxssh.PutFiles(td.Ctx, td.Hst, map[string]string{
		filepath.Join(srcDir, "file1"):             filepath.Join(dstDir, "newfile1"),           // rename to preserve orig file
		filepath.Join(srcDir, "dir/file2"):         filepath.Join(dstDir, "dir/file2"),          // overwrite orig file
		filepath.Join(srcDir, "dir2/subdir/file3"): filepath.Join(dstDir, "dir2/subdir2/file3"), // rename subdir
		filepath.Join(srcDir, weirdSrcName):        filepath.Join(dstDir, "file5"),              // check that regexp chars are escaped
		filepath.Join(srcDir, "file6"):             filepath.Join(dstDir, weirdDstName),         // check that replacement chars are also escaped
	}, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	} else if n <= 0 {
		t.Errorf("Copied non-positive %v bytes", n)
	}

	if err := checkDir(dstDir, map[string]string{
		"file1":              "file1 old",
		"newfile1":           "file1 new",
		"dir/file2":          "file2 new",
		"dir2/subdir/file3":  "file3 old",
		"dir2/subdir2/file3": "file3 new",
		"file4":              "file4 old",
		"file5":              "file5 new",
		weirdDstName:         "file6 new",
	}); err != nil {
		t.Error(err)
	}
}

func TestPutFilesUnchanged(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{
		"src1":        "1",
		"dir/src2":    "2",
		"dir 2/src 3": "3",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if err := testutil.WriteFiles(dstDir, map[string]string{
		"dst1":        "1",
		"dir/dst2":    "2",
		"dir 2/dst 3": "3",
	}); err != nil {
		t.Fatal(err)
	}

	// No bytes should be sent since the dest dir already contains the renamed source files.
	if n, err := linuxssh.PutFiles(td.Ctx, td.Hst, map[string]string{
		filepath.Join(srcDir, "src1"):        filepath.Join(dstDir, "dst1"),
		filepath.Join(srcDir, "dir/src2"):    filepath.Join(dstDir, "dir/dst2"),
		filepath.Join(srcDir, "dir 2/src 3"): filepath.Join(dstDir, "dir 2/dst 3"),
	}, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Errorf("PutFiles() copied %v bytes; want 0", n)
	}
}

func TestPutFilesTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)
	dstDir := filepath.Join(tmpDir, "dst")
	td.ExecTimeout = sshtest.EndTimeout
	if _, err := linuxssh.PutFiles(td.Ctx, td.Hst, map[string]string{
		filepath.Join(srcDir, "file"): filepath.Join(dstDir, "file"),
	}, linuxssh.PreserveSymlinks); err == nil {
		t.Errorf("PutFiles() with expired context didn't return error")
	}
}

func TestPutFilesSymlinks(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	tmpDir, srcDir := initFileTest(t, nil)
	defer os.RemoveAll(tmpDir)

	// Create a symlink pointing to an executable file outside of the source dir.
	const (
		data = "executable file"
		link = "symlink"
		mode = 0755
	)
	targetPath := filepath.Join(tmpDir, "exe")
	if err := ioutil.WriteFile(targetPath, []byte(data), mode); err != nil {
		t.Fatalf("Failed to write %v: %v", targetPath, err)
	}
	if err := os.Symlink(targetPath, filepath.Join(srcDir, link)); err != nil {
		t.Fatalf("Failed to create symlink to %v: %v", targetPath, err)
	}

	// PreserveSymlinks should copy symlinks directly.
	dstDir := filepath.Join(tmpDir, "dst_preserve")
	if _, err := linuxssh.PutFiles(td.Ctx, td.Hst, map[string]string{
		filepath.Join(srcDir, link): filepath.Join(dstDir, link),
	}, linuxssh.PreserveSymlinks); err != nil {
		t.Error("PutFiles failed with linuxssh.PreserveSymlinks: ", err)
	} else {
		dstFile := filepath.Join(dstDir, link)
		if fi, err := os.Lstat(dstFile); err != nil {
			t.Errorf("Failed to lstat %v: %v", dstFile, err)
		} else if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%v is not a symlink; got mode %v", dstFile, fi.Mode())
		} else if target, err := os.Readlink(dstFile); err != nil {
			t.Errorf("Failed to read %v: %v", dstFile, err)
		} else if target != targetPath {
			t.Errorf("%v has target %v; want %v", dstFile, target, targetPath)
		}
	}

	// DereferenceSymlinks should copy symlinks' targets while preserving the original mode.
	dstDir = filepath.Join(tmpDir, "dst_deref")
	if _, err := linuxssh.PutFiles(td.Ctx, td.Hst, map[string]string{
		filepath.Join(srcDir, link): filepath.Join(dstDir, link),
	}, linuxssh.DereferenceSymlinks); err != nil {
		t.Error("PutFiles failed with DereferenceSymlinks: ", err)
	} else {
		dstFile := filepath.Join(dstDir, link)
		if fi, err := os.Lstat(dstFile); err != nil {
			t.Errorf("Failed to lstat %v: %v", dstFile, err)
		} else if fi.Mode()&os.ModeType != 0 {
			t.Errorf("%v is not a regular file; got mode %v", dstFile, fi.Mode())
		} else if perm := fi.Mode() & os.ModePerm; perm != mode {
			t.Errorf("%v has perms %#o; want %#o", dstFile, perm, mode)
		}
		if b, err := ioutil.ReadFile(dstFile); err != nil {
			t.Errorf("Failed to read %v: %v", dstFile, err)
		} else if string(b) != data {
			t.Errorf("%v contains %q; want %q", dstFile, b, data)
		}
	}
}

func TestDeleteTree(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{
		"file1":     "first file",
		"file2":     "second file",
		"dir/file3": "third file",
		"dir/file4": "fourth file",
	}
	tmpDir, baseDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	if err := linuxssh.DeleteTree(td.Ctx, td.Hst, baseDir, []string{"file1", "dir", "file9"}); err != nil {
		t.Fatal(err)
	}

	expected := map[string]string{"file2": "second file"}
	if err := checkDir(baseDir, expected); err != nil {
		t.Error(err)
	}
}

func TestDeleteTreeOutside(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	tmpDir, baseDir := initFileTest(t, nil)
	defer os.RemoveAll(tmpDir)

	if err := linuxssh.DeleteTree(td.Ctx, td.Hst, baseDir, []string{"dir/../../outside"}); err == nil {
		t.Error("DeleteTree succeeded; should fail")
	}
}

func TestGetFilePerms(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"rofile": "read only", "rwfile": "read write", "exec": "read and exec"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	for _, f := range []struct {
		filename string
		perm     os.FileMode
	}{
		{"rofile", 0444},
		{"rwfile", 0666},
		{"exec", 0555},
	} {
		if err := os.Chmod(filepath.Join(srcDir, f.filename), f.perm); err != nil {
			t.Fatal(err)
		}

		srcFile := filepath.Join(srcDir, f.filename)
		dstFile := filepath.Join(tmpDir, f.filename+".copy")
		if err := linuxssh.GetFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(dstFile)
		if err != nil {
			t.Error(err)
		}
		if info.Mode().Perm() != f.perm {
			t.Errorf("File %s should have perms %#o but was %#o", dstFile, f.perm, info.Mode().Perm())
		}
	}
}

func TestPutFilesPerm(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"rofile": "read only", "rwfile": "read write", "exec": "read and exec"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	fileperms := []struct {
		filename string
		perm     os.FileMode
	}{
		{"rofile", 0444},
		{"rwfile", 0666},
		{"exec", 0555},
	}

	for _, f := range fileperms {
		if err := os.Chmod(filepath.Join(srcDir, f.filename), f.perm); err != nil {
			t.Fatal(err)
		}
	}

	dstDir := filepath.Join(tmpDir, "dst")
	if n, err := linuxssh.PutFiles(td.Ctx, td.Hst, map[string]string{
		filepath.Join(srcDir, "rofile"): filepath.Join(dstDir, "rofile"),
		filepath.Join(srcDir, "rwfile"): filepath.Join(dstDir, "rwfile"),
		filepath.Join(srcDir, "exec"):   filepath.Join(dstDir, "exec"),
	}, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	} else if n <= 0 {
		t.Errorf("Copied non-positive %v bytes", n)
	}

	for _, f := range fileperms {
		info, err := os.Stat(filepath.Join(dstDir, f.filename))
		if err != nil {
			t.Error(err)
		}
		if info.Mode().Perm() != f.perm {
			t.Errorf("File %s should have perms %#o but was %#o", f.filename, f.perm, info.Mode().Perm())
		}
	}
}

func TestGetAndDeleteFile(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "foo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := linuxssh.GetAndDeleteFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}

	// TestGetAndDeleteFile should have removed the original file.
	if _, err := os.Stat(srcFile); err == nil {
		t.Error("GetAndDeleteFile did not delete a file")
	} else if !os.IsNotExist(err) {
		t.Error(err)
	}
}

func TestGetAndDeleteFilesInDir(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{
		"dir/a": "a",
		"b":     "b",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if err := testutil.WriteFiles(dstDir, map[string]string{"c": "c"}); err != nil {
		t.Fatal(err)
	}
	if err := linuxssh.GetAndDeleteFilesInDir(td.Ctx, td.Hst, srcDir, dstDir, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	m, err := testutil.ReadFiles(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(m, map[string]string{
		"dir/a": "a",
		"b":     "b",
		"c":     "c",
	}); diff != "" {
		t.Errorf("Files mismatch (-got +want):\n%v", diff)
	}
	// TestGetAndDeleteFile should have removed the original file.
	if _, err := os.Stat(srcDir); err == nil {
		t.Error("GetAndDeleteFile did not delete a file")
	} else if !os.IsNotExist(err) {
		t.Error(err)
	}
}

func TestGetAndDeleteFilesInDirMakesDirectory(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	tmpDir, srcDir := initFileTest(t, map[string]string{})
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if _, err := os.Stat(dstDir); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(%q) = %v, want not exist", dstDir, err)
	}

	if err := linuxssh.GetAndDeleteFilesInDir(td.Ctx, td.Hst, srcDir, dstDir, linuxssh.PreserveSymlinks); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dstDir); err != nil || !info.IsDir() {
		t.Errorf("GetAndDeleteFile did not create a directory: %v", err)
	}
}
