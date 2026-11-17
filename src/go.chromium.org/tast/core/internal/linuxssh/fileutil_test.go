// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxssh_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/internal/linuxssh"
	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testutil"
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
	if b, err := os.ReadFile(p); err != nil {
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

func TestGetFileTail(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "fooooooooo", "fileWanted": "oooo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := linuxssh.GetFileTail(td.Ctx, td.Hst, srcFile, dstFile, 1, 4); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["fileWanted"]); err != nil {
		t.Error(err)
	}

	// GetFileTail should overwrite local files.
	if err := linuxssh.GetFileTail(td.Ctx, td.Hst, srcFile, dstFile, 1, 4); err != nil {
		t.Error(err)
	}
}
func TestGetFileTailWithStartLine(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	files := map[string]string{"file": "line1\nline2\nline3\n", "fileWanted": "line2\nline3\n"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := linuxssh.GetFileTail(td.Ctx, td.Hst, srcFile, dstFile, 2, 500); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["fileWanted"]); err != nil {
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
	if err := os.WriteFile(targetPath, []byte(data), mode); err != nil {
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
		if b, err := os.ReadFile(dstFile); err != nil {
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

type recordingSSH struct {
	hst        *ssh.Conn
	mu         sync.Mutex
	cmds       []string
	beforeExec func(cmd string)
}

// newRecordingSSH starts a local SSH server that records every executed command.
func newRecordingSSH(t *testing.T) *recordingSSH {
	t.Helper()
	userKey, hostKey := sshtest.MustGenerateKeys()

	rec := &recordingSSH{}
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(req *sshtest.ExecReq) {
		rec.mu.Lock()
		rec.cmds = append(rec.cmds, req.Cmd)
		hook := rec.beforeExec
		rec.mu.Unlock()
		if hook != nil {
			hook(req.Cmd)
		}
		req.Start(true)
		req.End(req.RunRealCmd())
	})
	if err != nil {
		t.Fatal(err)
	}
	hst, err := sshtest.ConnectToServer(t.Context(), srv, userKey, &ssh.Options{})
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	rec.hst = hst
	t.Cleanup(func() {
		srv.Close()
		hst.Close(context.Background())
	})
	return rec
}

func (r *recordingSSH) setBeforeExec(fn func(string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeExec = fn
}

func (r *recordingSSH) takeCmds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.cmds
	r.cmds = nil
	return out
}

func TestGetAndDeleteSingleCommand(t *testing.T) {
	t.Parallel()
	rec := newRecordingSSH(t)

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	// 1. Regular file (including read-only 0444 mode).
	srcFile := filepath.Join(tmpDir, "src_file")
	if err := os.WriteFile(srcFile, []byte("hello"), 0444); err != nil {
		t.Fatal(err)
	}
	dstFile := filepath.Join(tmpDir, "dst_file")
	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFile(file) failed: %v", err)
	}
	if err := checkFile(dstFile, "hello"); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(srcFile); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", srcFile, err)
	}
	if cmds := rec.takeCmds(); len(cmds) != 1 {
		t.Errorf("GetAndDeleteFile(file) ran %d SSH commands (%q); want 1", len(cmds), cmds)
	}

	// 2. Top-level relative symlink (dangling inside temporary extraction dir td).
	relTarget := filepath.Join(tmpDir, "rel_target.txt")
	if err := os.WriteFile(relTarget, []byte("target-body"), 0644); err != nil {
		t.Fatal(err)
	}
	relLinkSrc := filepath.Join(tmpDir, "rel_link_src")
	if err := os.Symlink("rel_target.txt", relLinkSrc); err != nil {
		t.Fatal(err)
	}
	relLinkDst := filepath.Join(tmpDir, "rel_link_dst")
	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, relLinkSrc, relLinkDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFile(relative symlink) failed: %v", err)
	}
	if got, err := os.Readlink(relLinkDst); err != nil || got != "rel_target.txt" {
		t.Errorf("os.Readlink(%q) = (%q, %v); want (%q, nil)", relLinkDst, got, err, "rel_target.txt")
	}
	if _, err := os.Lstat(relLinkSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", relLinkSrc, err)
	}
	if err := checkFile(relTarget, "target-body"); err != nil {
		t.Errorf("Relative symlink target should remain intact: %v", err)
	}
	if cmds := rec.takeCmds(); len(cmds) != 1 {
		t.Errorf("GetAndDeleteFile(relative symlink) ran %d SSH commands (%q); want 1", len(cmds), cmds)
	}

	// 3. Nested directory with files, empty subdirectory, read-only file, and symlinks.
	nestedSrc := filepath.Join(tmpDir, "nested_src")
	nestedFiles := map[string]string{
		"a.txt":         "alpha",
		"sub/deep/b.go": "beta",
		"ro.txt":        "readonly",
	}
	if err := testutil.WriteFiles(nestedSrc, nestedFiles); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(nestedSrc, "ro.txt"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(nestedSrc, "emptysub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(nestedSrc, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(nestedSrc, "broken_link")); err != nil {
		t.Fatal(err)
	}
	nestedDst := filepath.Join(tmpDir, "nested_dst")
	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, nestedSrc, nestedDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFile(nested dir) failed: %v", err)
	}
	for rel, want := range nestedFiles {
		if err := checkFile(filepath.Join(nestedDst, rel), want); err != nil {
			t.Error(err)
		}
	}
	if fi, err := os.Stat(filepath.Join(nestedDst, "emptysub")); err != nil || !fi.IsDir() {
		t.Errorf("emptysub stat = (%v, %v); want directory", fi, err)
	}
	for name, want := range map[string]string{"link": "a.txt", "broken_link": "missing"} {
		if got, err := os.Readlink(filepath.Join(nestedDst, name)); err != nil || got != want {
			t.Errorf("os.Readlink(%q) = (%q, %v); want (%q, nil)", name, got, err, want)
		}
	}
	if _, err := os.Lstat(nestedSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", nestedSrc, err)
	}
	if cmds := rec.takeCmds(); len(cmds) != 1 {
		t.Errorf("GetAndDeleteFile(nested dir) ran %d SSH commands (%q); want 1", len(cmds), cmds)
	}

	// 4. Empty directory via GetAndDeleteFile.
	emptySrc := filepath.Join(tmpDir, "empty_src")
	if err := os.Mkdir(emptySrc, 0755); err != nil {
		t.Fatal(err)
	}
	emptyDst := filepath.Join(tmpDir, "empty_dst")
	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, emptySrc, emptyDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFile(empty dir) failed: %v", err)
	}
	if fi, err := os.Stat(emptyDst); err != nil || !fi.IsDir() {
		t.Errorf("emptyDst stat = (%v, %v); want directory", fi, err)
	}
	if _, err := os.Lstat(emptySrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", emptySrc, err)
	}
	if cmds := rec.takeCmds(); len(cmds) != 1 {
		t.Errorf("GetAndDeleteFile(empty dir) ran %d SSH commands (%q); want 1", len(cmds), cmds)
	}

	// 5. Nested and empty directories via GetAndDeleteFilesInDir.
	inDirSrc := filepath.Join(tmpDir, "indir_src")
	if err := testutil.WriteFiles(inDirSrc, map[string]string{"sub/x": "1", "y": "2"}); err != nil {
		t.Fatal(err)
	}
	inDirDst := filepath.Join(tmpDir, "indir_dst")
	if err := linuxssh.GetAndDeleteFilesInDir(t.Context(), rec.hst, inDirSrc, inDirDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFilesInDir(nested) failed: %v", err)
	}
	if err := checkDir(inDirDst, map[string]string{"sub/x": "1", "y": "2"}); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(inDirSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", inDirSrc, err)
	}
	if cmds := rec.takeCmds(); len(cmds) != 1 {
		t.Errorf("GetAndDeleteFilesInDir(nested) ran %d SSH commands (%q); want 1", len(cmds), cmds)
	}

	emptyInDirSrc := filepath.Join(tmpDir, "empty_indir_src")
	if err := os.Mkdir(emptyInDirSrc, 0755); err != nil {
		t.Fatal(err)
	}
	emptyInDirDst := filepath.Join(tmpDir, "empty_indir_dst")
	if err := linuxssh.GetAndDeleteFilesInDir(t.Context(), rec.hst, emptyInDirSrc, emptyInDirDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFilesInDir(empty) failed: %v", err)
	}
	if fi, err := os.Stat(emptyInDirDst); err != nil || !fi.IsDir() {
		t.Errorf("os.Stat(%q) = (%v, %v); want directory", emptyInDirDst, fi, err)
	}
	if err := checkDir(emptyInDirDst, map[string]string{}); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(emptyInDirSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", emptyInDirSrc, err)
	}
	if cmds := rec.takeCmds(); len(cmds) != 1 {
		t.Errorf("GetAndDeleteFilesInDir(empty) ran %d SSH commands (%q); want 1", len(cmds), cmds)
	}
}

func TestGetAndDeleteFallback(t *testing.T) {
	t.Parallel()
	rec := newRecordingSSH(t)

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	// 1. DereferenceSymlinks skips --remove-files and falls back to rm -rf.
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(target, []byte("deref-data"), 0644); err != nil {
		t.Fatal(err)
	}
	linkSrc := filepath.Join(tmpDir, "link_src")
	if err := os.Symlink(target, linkSrc); err != nil {
		t.Fatal(err)
	}
	linkDst := filepath.Join(tmpDir, "link_dst")
	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, linkSrc, linkDst, linuxssh.DereferenceSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFile(DereferenceSymlinks) failed: %v", err)
	}
	if err := checkFile(linkDst, "deref-data"); err != nil {
		t.Error(err)
	}
	if fi, err := os.Lstat(linkDst); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("linkDst Lstat = (%v, %v); want regular file", fi, err)
	}
	if _, err := os.Lstat(linkSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", linkSrc, err)
	}
	if err := checkFile(target, "deref-data"); err != nil {
		t.Errorf("Dereference target should remain intact: %v", err)
	}
	cmds := rec.takeCmds()
	if len(cmds) != 2 || !strings.Contains(cmds[0], "--dereference") || strings.Contains(cmds[0], "--remove-files") || !strings.HasPrefix(cmds[1], "exec rm -rf -- ") {
		t.Errorf("GetAndDeleteFile(DereferenceSymlinks) cmds = %q; want [tar --dereference without --remove-files, exec rm -rf -- ...]", cmds)
	}

	derefDirSrc := filepath.Join(tmpDir, "deref_dir_src")
	if err := os.Mkdir(derefDirSrc, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(derefDirSrc, "linked")); err != nil {
		t.Fatal(err)
	}
	derefDirDst := filepath.Join(tmpDir, "deref_dir_dst")
	if err := linuxssh.GetAndDeleteFilesInDir(t.Context(), rec.hst, derefDirSrc, derefDirDst, linuxssh.DereferenceSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFilesInDir(DereferenceSymlinks) failed: %v", err)
	}
	if err := checkDir(derefDirDst, map[string]string{"linked": "deref-data"}); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(derefDirSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", derefDirSrc, err)
	}
	if err := checkFile(target, "deref-data"); err != nil {
		t.Errorf("Dereference target should remain intact: %v", err)
	}
	cmds = rec.takeCmds()
	if len(cmds) != 2 || !strings.Contains(cmds[0], "--dereference") || strings.Contains(cmds[0], "--remove-files") || !strings.HasPrefix(cmds[1], "exec rm -rf -- ") {
		t.Errorf("GetAndDeleteFilesInDir(DereferenceSymlinks) cmds = %q; want [tar --dereference without --remove-files, exec rm -rf -- ...]", cmds)
	}

	if os.Geteuid() == 0 {
		t.Log("Skipping read-only directory fallback cases when running as root")
		return
	}

	// 2. Read-only (0555) non-empty subdirectory prevents non-root tar --remove-files
	// from unlinking its child, triggering fallback rm -rf (or returning "delete failed"
	// if fallback rm -rf also fails).
	roParentSrc := filepath.Join(tmpDir, "ro_parent_src")
	roSubSrc := filepath.Join(roParentSrc, "rosub")
	if err := testutil.WriteFiles(roParentSrc, map[string]string{"rosub/file": "locked"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roSubSrc, 0555); err != nil {
		t.Fatal(err)
	}
	roFailDst := filepath.Join(tmpDir, "ro_fail_dst")
	roParentDst := filepath.Join(tmpDir, "ro_parent_dst")
	defer func() {
		os.Chmod(roSubSrc, 0755)
		os.Chmod(filepath.Join(roFailDst, "rosub"), 0755)
		os.Chmod(filepath.Join(roParentDst, "rosub"), 0755)
	}()

	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, roParentSrc, roFailDst, linuxssh.PreserveSymlinks); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Errorf("GetAndDeleteFile(0555 subdir unremovable) = %v; want error containing %q", err, "delete failed")
	}
	cmds = rec.takeCmds()
	if len(cmds) != 2 || !strings.Contains(cmds[0], "--remove-files") || !strings.HasPrefix(cmds[1], "exec rm -rf -- ") {
		t.Errorf("GetAndDeleteFile(0555 subdir unremovable) cmds = %q; want [tar --remove-files, exec rm -rf -- ...]", cmds)
	}

	// Grant write permission right before fallback rm -rf runs (simulating root
	// on a DUT after tar --remove-files fails to unlink).
	rec.setBeforeExec(func(cmd string) {
		if strings.HasPrefix(cmd, "exec rm -rf -- ") {
			os.Chmod(roSubSrc, 0755)
		}
	})

	if err := linuxssh.GetAndDeleteFile(t.Context(), rec.hst, roParentSrc, roParentDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFile(0555 subdir fallback) failed: %v", err)
	}
	rec.setBeforeExec(nil)

	if err := checkFile(filepath.Join(roParentDst, "rosub/file"), "locked"); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(roParentSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", roParentSrc, err)
	}
	cmds = rec.takeCmds()
	if len(cmds) != 2 || !strings.Contains(cmds[0], "--remove-files") || !strings.HasPrefix(cmds[1], "exec rm -rf -- ") {
		t.Errorf("GetAndDeleteFile(0555 subdir fallback) cmds = %q; want [tar --remove-files, exec rm -rf -- ...]", cmds)
	}

	// 3. Read-only (0555) parent directory prevents tar --remove-files from rmdir-ing
	// src itself while keeping src writable (0755) so GetAndDeleteFilesInDir can move
	// extracted files locally before falling back to rm -rf.
	roHolder := filepath.Join(tmpDir, "ro_holder")
	roInDirSrc := filepath.Join(roHolder, "indir_src")
	if err := testutil.WriteFiles(roInDirSrc, map[string]string{"file": "locked2"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roHolder, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(roHolder, 0755)

	roInDirFailDst := filepath.Join(tmpDir, "ro_indir_fail_dst")
	if err := linuxssh.GetAndDeleteFilesInDir(t.Context(), rec.hst, roInDirSrc, roInDirFailDst, linuxssh.PreserveSymlinks); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Errorf("GetAndDeleteFilesInDir(0555 parent unremovable) = %v; want error containing %q", err, "delete failed")
	}
	cmds = rec.takeCmds()
	if len(cmds) != 2 || !strings.Contains(cmds[0], "--remove-files") || !strings.HasPrefix(cmds[1], "exec rm -rf -- ") {
		t.Errorf("GetAndDeleteFilesInDir(0555 parent unremovable) cmds = %q; want [tar --remove-files, exec rm -rf -- ...]", cmds)
	}

	// Recreate file since the first tar --remove-files unlinked it inside writable
	// roInDirSrc before failing to rmdir roInDirSrc under read-only roHolder.
	if err := os.WriteFile(filepath.Join(roInDirSrc, "file"), []byte("locked2"), 0644); err != nil {
		t.Fatal(err)
	}
	rec.setBeforeExec(func(cmd string) {
		if strings.HasPrefix(cmd, "exec rm -rf -- ") {
			os.Chmod(roHolder, 0755)
		}
	})

	roInDirDst := filepath.Join(tmpDir, "ro_indir_dst")
	if err := linuxssh.GetAndDeleteFilesInDir(t.Context(), rec.hst, roInDirSrc, roInDirDst, linuxssh.PreserveSymlinks); err != nil {
		t.Fatalf("GetAndDeleteFilesInDir(0555 parent fallback) failed: %v", err)
	}
	rec.setBeforeExec(nil)

	if err := checkDir(roInDirDst, map[string]string{"file": "locked2"}); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(roInDirSrc); !os.IsNotExist(err) {
		t.Errorf("os.Lstat(%q) = %v; want not exist", roInDirSrc, err)
	}
	cmds = rec.takeCmds()
	if len(cmds) != 2 || !strings.Contains(cmds[0], "--remove-files") || !strings.HasPrefix(cmds[1], "exec rm -rf -- ") {
		t.Errorf("GetAndDeleteFilesInDir(0555 parent fallback) cmds = %q; want [tar --remove-files, exec rm -rf -- ...]", cmds)
	}
}

func TestGetAndDeleteFileEndTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	tmpDir, srcDir := initFileTest(t, map[string]string{"file": "data"})
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "dst")
	td.ExecTimeout = sshtest.EndTimeout
	if err := linuxssh.GetAndDeleteFile(td.Ctx, td.Hst, srcFile, dstFile, linuxssh.PreserveSymlinks); err == nil {
		t.Error("GetAndDeleteFile() with context canceled before exit-status = nil; want error")
	}
}

func TestGetFileNonexistent(t *testing.T) {
	t.Parallel()
	rec := newRecordingSSH(t)

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	missing := filepath.Join(tmpDir, "does_not_exist")
	cases := []struct {
		name string
		dst  string
		fn   func(context.Context, *ssh.Conn, string, string, linuxssh.SymlinkPolicy) error
	}{
		{"GetFile", filepath.Join(tmpDir, "dst_get"), linuxssh.GetFile},
		{"GetAndDeleteFile", filepath.Join(tmpDir, "dst_get_del"), linuxssh.GetAndDeleteFile},
		{"GetAndDeleteFilesInDir", filepath.Join(tmpDir, "dst_in_dir"), linuxssh.GetAndDeleteFilesInDir},
	}
	for _, policy := range []linuxssh.SymlinkPolicy{linuxssh.PreserveSymlinks, linuxssh.DereferenceSymlinks} {
		for _, tc := range cases {
			err := tc.fn(t.Context(), rec.hst, missing, tc.dst, policy)
			if err == nil || !strings.Contains(err.Error(), "running remote tar failed") {
				t.Errorf("%s(%q, %v) = %v; want error containing %q", tc.name, missing, policy, err, "running remote tar failed")
			}
			if cmds := rec.takeCmds(); len(cmds) != 1 {
				t.Errorf("%s(%q, %v) ran %d commands (%q); want 1", tc.name, missing, policy, len(cmds), cmds)
			}
		}
	}
	if entries, err := os.ReadDir(tmpDir); err != nil || len(entries) != 0 {
		t.Errorf("os.ReadDir(%q) = (%v, %v); want empty directory", tmpDir, entries, err)
	}
}
