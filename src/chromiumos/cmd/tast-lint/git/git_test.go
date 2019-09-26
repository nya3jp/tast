// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package git

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"chromiumos/tast/testutil"
)

const (
	staticName    = "static.txt"
	testName      = "test.txt"
	deleteName    = "delete.txt"
	newName       = "new.txt"
	untrackedName = "untracked.txt"

	headContent = "foo"
	workContent = "bar"
)

// newTestRepo creates a new Git working tree for testing and returns the
// directory path. The repository will contain two commits:
//
//  In the first commit:
//    static.txt = "static"
//    test.txt = ""
// 		delete.txt = ""
//
//  In the second commit:
//    static.txt = "static"
//    test.txt = "foo"
//		new.txt = "baz"
//
//  In the work tree:
//    static.txt = "static"
//    test.txt = "bar"
// 		new.txt = "baz"
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
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := New(repoDir, "HEAD")
	fns, err := g.ChangedFiles()
	if err != nil {
		t.Fatal("ChangedFiles failed: ", err)
	}
	if exp := []CommitFile{CommitFile{Deleted, deleteName}, CommitFile{Added, newName}, CommitFile{Modified, testName}}; !reflect.DeepEqual(fns, exp) {
		t.Errorf("ChangedFiles() = %q; want %q", fns, exp)
	}
}

func TestChangedFilesInWorkTree(t *testing.T) {
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := New(repoDir, "")
	if _, err := g.ChangedFiles(); err == nil {
		t.Error("ChangedFiles unexpectedly succeeded")
	}
}

func TestReadFileInHistory(t *testing.T) {
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := New(repoDir, "HEAD")

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
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := New(repoDir, "")

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
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := New(repoDir, "HEAD")

	if fns, err := g.ListDir(""); err != nil {
		t.Errorf("ListDir(%q) failed: %v", "", err)
	} else if exp := []string{newName, staticName, testName}; !reflect.DeepEqual(fns, exp) {
		t.Errorf("ListDir(%q) = %q; want %q", "", fns, exp)
	}

	if _, err := g.ListDir(testName); err == nil {
		t.Errorf("ListDir(%q) unexpectedly succeeded", testName)
	}
}

func TestListDirInWorkTree(t *testing.T) {
	repoDir := newTestRepo(t)
	defer os.RemoveAll(repoDir)

	g := New(repoDir, "")

	if fns, err := g.ListDir(""); err != nil {
		t.Errorf("ListDir(%q) failed: %v", "", err)
	} else if exp := []string{".git", newName, staticName, testName, untrackedName}; !reflect.DeepEqual(fns, exp) {
		t.Errorf("ListDir(%q) = %q; want %q", "", fns, exp)
	}

	if _, err := g.ListDir(testName); err == nil {
		t.Errorf("ListDir(%q) unexpectedly succeeded", testName)
	}
}
