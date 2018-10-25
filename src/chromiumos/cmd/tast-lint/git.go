// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
)

// git is a thin wrapper of git command line tool allowing to access files in Git history.
type git struct {
	// commit is the hash of a commit to operate on. If empty, it operates on the working tree.
	commit string
}

// newGit creates a git object operating on a commit identified by commit.
// If commit is empty, it operates on the working tree.
func newGit(commit string) *git {
	return &git{commit}
}

// modifiedFiles returns the list of file paths modified in the commit.
func (g *git) modifiedFiles() ([]string, error) {
	if g.commit == "" {
		return nil, errors.New("modifiedFiles needs explicit commit")
	}
	out, err := exec.Command("git", "diff-tree", "--no-commit-id", "-r", "--name-only", g.commit).Output()
	if err != nil {
		return nil, err
	}
	files := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	return files, nil
}

// readFile returns the content of a file at the commit.
func (g *git) readFile(path string) ([]byte, error) {
	if g.commit == "" {
		return ioutil.ReadFile(path)
	}
	return exec.Command("git", "show", fmt.Sprintf("%s:%s", g.commit, path)).Output()
}

// listDir lists files under a directory at the commit.
func (g *git) listDir(dir string) ([]string, error) {
	if g.commit == "" {
		fs, err := ioutil.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, f := range fs {
			names = append(names, f.Name())
		}
		return names, nil
	}

	cmd := exec.Command("git", "ls-tree", "--name-only", fmt.Sprintf("%s:%s", g.commit, dir))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}
