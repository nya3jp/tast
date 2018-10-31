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

// git is a thin wrapper of git command line tool.
type git struct {
	commit string // commit to operate on; if empty, operate on the working tree
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
	out, err := exec.Command("git", "diff-tree", "--no-commit-id", "-r", "--name-status", g.commit).Output()
	if err != nil {
		return nil, err
	}
	stats := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	files := []string{}
	for _, s := range stats {
		if parts := strings.Split(s, "\t"); len(parts) == 2 && parts[0] != "D" {
			files = append(files, parts[1])
		}
	}
	return files, nil
}

// readFile returns the content of a file in the commit.
func (g *git) readFile(path string) ([]byte, error) {
	if g.commit == "" {
		return ioutil.ReadFile(path)
	}
	return exec.Command("git", "show", fmt.Sprintf("%s:%s", g.commit, path)).Output()
}
