// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package git_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"go.chromium.org/tast/cmd/tast-lint/internal/git"
	"go.chromium.org/tast/testutil"
)

const (
	staticName    = "static.txt"
	testName      = "test.txt"
	deleteName    = "delete.txt"
	newName       = "new.txt"
	untrackedName = "untracked.txt"
	symlinkName   = "symlink.txt"

	headContent = "foo"
	workContent = "bar"
)

// newTestRepo creates a new Git working tree for testing and returns the
// directory path. The repository will contain two commits:
//
//  In the first commit:
//    static.txt = "static"
//    test.txt = ""
//    delete.txt = ""
//
//  In the second commit:
//    static.txt = "static"
//    test.txt = "foo"
//    new.txt = "baz"
//    symlink.txt = symlink to ./static.txt
//
//  In the work tree:
//    static.txt = "static"
//    test.txt = "bar"
//    new.txt = "baz"
//    symlink.txt = symlink to ./static.txt
//    untracked.txt = ""
func newTestRepo(t *testing.T) string {
	t.Helper()

	repoDir := testutil.TempDir(t)
	success := false
	defer func() {
		if !success {
			os.RemoveAll(repoDir)
		}
	}()

	if err := exec.Command("git", "init", repoDir).Run(); err != nil {
		t.Fatal("git init failed: ", err)
	}

	for _, kv := range []struct {
		key, value string
	}{
		{"user.name", "me"},
		{"user.email", "me@example.com"},
	} {
		cmd := exec.Command("git", "config", "--local", kv.key, kv.value)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			t.Fatal("git config failed: ", err)
		}
	}

	// Create the first commit.
	if err := ioutil.WriteFile(filepath.Join(repoDir, staticName), []byte("static"), 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}
	testPath := filepath.Join(repoDir, testName)
	if err := ioutil.WriteFile(testPath, nil, 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}
	if err := ioutil.WriteFile(filepath.Join(repoDir, deleteName), nil, 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}

	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal("git add failed: ", err)
	}

	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal("git commit failed: ", err)
	}

	// Create the second commit.
	if err := ioutil.WriteFile(testPath, []byte(headContent), 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}
	if err := ioutil.WriteFile(filepath.Join(repoDir, newName), []byte("baz"), 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}
	if err := os.Symlink(filepath.Join(".", staticName), filepath.Join(repoDir, symlinkName)); err != nil {
		t.Fatal("Set up new repo:", err)
	}

	cmd = exec.Command("git", "rm", deleteName)
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal("git add failed: ", err)
	}

	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal("git add failed: ", err)
	}

	cmd = exec.Command("git", "commit", "-a", "-m", "hello")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal("git commit failed: ", err)
	}

	// Create the work tree.
	if err := ioutil.WriteFile(testPath, []byte(workContent), 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}
	if err := ioutil.WriteFile(filepath.Join(repoDir, untrackedName), nil, 0644); err != nil {
		t.Fatal("WriteFile failed: ", err)
	}

	success = true
	return repoDir
}

func TestChangedFilesInHistory(t *testing.T) {
	t.Parallel()
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, "HEAD")
	fns, err := g.ChangedFiles()
	if err != nil {
		t.Fatal("ChangedFiles failed: ", err)
	}
	if exp := []git.CommitFile{
		{git.Deleted, deleteName},
		{git.Added, newName},
		{git.Added, symlinkName},
		{git.Modified, testName},
	}; !reflect.DeepEqual(fns, exp) {
		t.Errorf("ChangedFiles() = %q; want %q", fns, exp)
	}
}

func TestChangedFilesInWorkTree(t *testing.T) {
	t.Parallel()
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, "")
	if _, err := g.ChangedFiles(); err == nil {
		t.Error("ChangedFiles unexpectedly succeeded")
	}
}

func TestReadFileInHistory(t *testing.T) {
	t.Parallel()
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, "HEAD")

	if out, err := g.ReadFile(testName); err != nil {
		t.Errorf("ReadFile(%q) failed: %v", testName, err)
	} else if s := string(out); s != headContent {
		t.Errorf("ReadFile(%q) = %q; want %q", testName, s, headContent)
	}

	if out, err := g.ReadFile(untrackedName); err == nil {
		t.Errorf("ReadFile(%q) unexpectedly succeeded; content=%q", untrackedName, out)
	}
}

func TestReadFileInWorkTree(t *testing.T) {
	t.Parallel()
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, "")

	if out, err := g.ReadFile(testName); err != nil {
		t.Errorf("ReadFile(%q) workContent: %v", testName, err)
	} else if s := string(out); s != workContent {
		t.Errorf("ReadFile(%q) = %q; want %q", testName, s, workContent)
	}

	const fn = "no_such_file"
	if _, err := g.ReadFile(fn); err == nil {
		t.Errorf("ReadFile(%q) unexpectedly succeeded", fn)
	}
}

func TestListDirInHistory(t *testing.T) {
	t.Parallel()
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, "HEAD")

	if fns, err := g.ListDir(""); err != nil {
		t.Errorf("ListDir(%q) failed: %v", "", err)
	} else if exp := []string{newName, staticName, symlinkName, testName}; !reflect.DeepEqual(fns, exp) {
		t.Errorf("ListDir(%q) = %q; want %q", "", fns, exp)
	}

	if _, err := g.ListDir(testName); err == nil {
		t.Errorf("ListDir(%q) unexpectedly succeeded", testName)
	}
}

func TestListDirInWorkTree(t *testing.T) {
	t.Parallel()
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, "")

	if fns, err := g.ListDir(""); err != nil {
		t.Errorf("ListDir(%q) failed: %v", "", err)
	} else if exp := []string{".git", newName, staticName, symlinkName, testName, untrackedName}; !reflect.DeepEqual(fns, exp) {
		t.Errorf("ListDir(%q) = %q; want %q", "", fns, exp)
	}

	if _, err := g.ListDir(testName); err == nil {
		t.Errorf("ListDir(%q) unexpectedly succeeded", testName)
	}
}

func TestIsSymlinkInHistory(t *testing.T) {
	t.Parallel()
	testIsSymlink(t, "HEAD")
}

func TestIsSmylinkInWorkTree(t *testing.T) {
	t.Parallel()
	testIsSymlink(t, "")
}

func testIsSymlink(t *testing.T, commit string) {
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := git.New(repoDir, commit)

	for _, tc := range []struct {
		file string
		want bool
	}{
		{staticName, false},
		{symlinkName, true},
	} {
		if got, err := g.IsSymlink(tc.file); err != nil {
			t.Errorf("IsSymlink(%q) failed: %v", tc.file, err)
		} else if got != tc.want {
			t.Errorf("IsSymlink(%q) = %v; want %v", tc.file, got, tc.want)
		}
	}
}
