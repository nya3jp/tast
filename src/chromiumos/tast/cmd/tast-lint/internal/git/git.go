// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package git operates git command.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
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

// CommitStatus represent the status of changed files.
type CommitStatus int

// Constants represent the commit status as an enumerated type.
const (
	Added CommitStatus = iota
	Copied
	Deleted
	Modified
	Renamed
	TypeChanged
	Unmerged
	Unknown
)

// CommitFile is a tuple composed of commit status and file path.
type CommitFile struct {
	Status CommitStatus
	Path   string
}

// ChangedFiles returns the list of pairs of commit statuses and file paths changed in the commit.
func (g *Git) ChangedFiles() ([]CommitFile, error) {
	if g.Commit == "" {
		return nil, errors.New("ChangedFiles needs explicit commit")
	}
	// TODO(nya): This does not work for the first, no-parent commit.
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "-r", "--name-status", g.Commit)
	cmd.Dir = g.Dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	stats := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var files []CommitFile
	for _, s := range stats {
		if parts := strings.Split(s, "\t"); len(parts) == 2 {
			switch status := parts[0]; status {
			case "A":
				files = append(files, CommitFile{Added, parts[1]})
			case "C":
				files = append(files, CommitFile{Copied, parts[1]})
			case "D":
				files = append(files, CommitFile{Deleted, parts[1]})
			case "M":
				files = append(files, CommitFile{Modified, parts[1]})
			case "R":
				files = append(files, CommitFile{Renamed, parts[1]})
			case "T":
				files = append(files, CommitFile{TypeChanged, parts[1]})
			case "U":
				files = append(files, CommitFile{Unmerged, parts[1]})
			default:
				files = append(files, CommitFile{Unknown, parts[1]})
				fmt.Println("files unknown (most probably a bug of git, please report it)")
			}
		}
	}
	return files, nil
}

// IsSymlink returns whether the path is a symlink.
func (g *Git) IsSymlink(path string) (bool, error) {
	if g.Commit == "" {
		s, err := os.Lstat(filepath.Join(g.Dir, path))
		if err != nil {
			return false, err
		}
		return (s.Mode() & os.ModeSymlink) != 0, nil
	}
	cmd := exec.Command("git", "ls-tree", g.Commit, path)
	cmd.Dir = g.Dir

	b, err := cmd.Output()
	if err != nil {
		return false, err
	}
	es := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(es) != 1 {
		return false, fmt.Errorf("%s matches %d files; want exactly one match", path, len(es))
	}
	return strings.SplitN(es[0], " ", 2)[0] == "120000", nil
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
