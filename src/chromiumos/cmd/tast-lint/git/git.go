// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package git

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git is a thin wrapper of git command line tool allowing to access files in Git history.
type Git struct {
	// Dir is the root directory of a Git repository.
	Dir string

	// Commit is the hash of a commit to operate on. If empty, it operates on the working tree.
	Commit string
}

// New creates a Git object operating on a commit identified by commit.
// If commit is empty, it operates on the working tree.
func New(dir, commit string) *Git {
	return &Git{
		Dir:    dir,
		Commit: commit,
	}
}

// ModifiedFiles returns the list of file paths modified in the commit.
func (g *Git) ModifiedFiles() ([]string, error) {
	if g.Commit == "" {
		return nil, errors.New("ModifiedFiles needs explicit commit")
	}
	// TODO(nya): This does not work for the first, no-parent commit.
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "-r", "--name-status", g.Commit)
	cmd.Dir = g.Dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	stats := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var files []string
	for _, s := range stats {
		if parts := strings.Split(s, "\t"); len(parts) == 2 && parts[0] != "D" {
			files = append(files, parts[1])
		}
	}
	return files, nil
}

// ReadFile returns the content of a file at the commit.
func (g *Git) ReadFile(path string) ([]byte, error) {
	if g.Commit == "" {
		return ioutil.ReadFile(filepath.Join(g.Dir, path))
	}

	// "--batch=" == use an empty format. Skip object information, just return the content.
	cmd := exec.Command("git", "cat-file", "--batch=", "--follow-symlinks")
	cmd.Dir = g.Dir
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", g.Commit, path))
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lf := []byte{'\n'}
	if !bytes.HasPrefix(b, lf) {
		msg := strings.Split(string(b), "\n")[0]
		return nil, fmt.Errorf("git cat-file failed: %s", msg)
	}
	// Skip LFs surrounding the content.
	return bytes.TrimSuffix(bytes.TrimPrefix(b, lf), lf), nil
}

// ListDir lists files under a directory at the commit.
func (g *Git) ListDir(dir string) ([]string, error) {
	if g.Commit == "" {
		fs, err := ioutil.ReadDir(filepath.Join(g.Dir, dir))
		if err != nil {
			return nil, err
		}
		var names []string
		for _, f := range fs {
			names = append(names, f.Name())
		}
		return names, nil
	}

	cmd := exec.Command("git", "ls-tree", "--name-only", fmt.Sprintf("%s:%s", g.Commit, dir))
	cmd.Dir = g.Dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}
