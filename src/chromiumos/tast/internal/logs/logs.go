// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package logs is used on-device to collect updates to system logs.
package logs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// InodeSizes maps from inode to file size.
type InodeSizes map[uint64]int64

// GetLogInodeSizes recursively walks dir and returns a map from inode
// to size in bytes for all regular files. warnings contains non-fatal errors
// that were accounted, keyed by path. Exclude lists relative paths of directories
// and files to skip.
func GetLogInodeSizes(dir string, exclude []string) (
	inodes InodeSizes, warnings map[string]error, err error) {
	inodes = make(InodeSizes)
	warnings = make(map[string]error)

	wf := func(p string, info os.FileInfo, err error) error {
		if err != nil {
			warnings[p] = err
			return nil
		}
		if skip, walkErr := shouldSkip(p, dir, info, exclude); skip {
			return walkErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			warnings[p] = fmt.Errorf("can't get inode for %s", p)
			return nil
		}
		inodes[stat.Ino] = info.Size()
		return nil
	}
	if err := filepath.Walk(dir, wf); err != nil {
		return nil, warnings, err
	}
	return inodes, warnings, nil
}

// CopyLogFileUpdates takes origSizes, the result of an earlier call to
// GetLogInodeSizes, and copies new parts of files under directory src to
// directory dst, creating it if necessary. The exclude arg lists relative
// paths of directories and files to skip. A nil or empty size map may be
// passed to copy all files in their entirety. warnings contains non-fatal
// errors that were accounted, keyed by path.
func CopyLogFileUpdates(src, dst string, exclude []string, origSizes InodeSizes) (
	warnings map[string]error, err error) {
	warnings = make(map[string]error)
	if err = os.MkdirAll(dst, 0755); err != nil {
		return warnings, err
	}

	err = filepath.Walk(src, func(sp string, info os.FileInfo, err error) error {
		if err != nil {
			warnings[sp] = err
			return nil
		}
		if skip, walkErr := shouldSkip(sp, src, info, exclude); skip {
			return walkErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			warnings[sp] = fmt.Errorf("can't get inode for %s", sp)
			return nil
		}
		var origSize int64
		if origSizes != nil {
			origSize = origSizes[stat.Ino]
		}
		if info.Size() == origSize {
			return nil
		}
		if info.Size() < origSize {
			warnings[sp] = fmt.Errorf("%s is shorter than original (now %d, original %d), copying all innstaead of diff", sp, info.Size(), origSize)
			origSize = 0
		}

		dp := filepath.Join(dst, sp[len(src):])
		if err = os.MkdirAll(filepath.Dir(dp), 0755); err != nil {
			return err
		}

		sf, err := os.Open(sp)
		if err != nil {
			warnings[sp] = err
			return nil
		}
		defer sf.Close()

		if _, err = sf.Seek(origSize, 0); err != nil {
			warnings[sp] = err
			return nil
		}

		df, err := os.Create(dp)
		if err != nil {
			return err
		}
		defer df.Close()

		if _, err = io.Copy(df, sf); err != nil {
			warnings[sp] = err
		}
		return nil
	})
	return warnings, err
}

// shouldSkip is a helper function called from a filepath.WalkFunc to check if the supplied absolute
// path should be skipped. root is the root path that was previously passed to filepath.Walk, fi
// corresponds to path, and exclude contains a list of paths relative to root to skip.
// If the returned skip value is true, then the calling filepath.WalkFunc should return walkErr.
func shouldSkip(path, root string, fi os.FileInfo, exclude []string) (skip bool, walkErr error) {
	if path == root {
		return false, nil
	}

	if !strings.HasPrefix(path, root+"/") {
		return true, fmt.Errorf("path %v not in root %v", path, root)
	}

	rel := path[len(root)+1:]
	for _, e := range exclude {
		if e == rel {
			if fi.IsDir() {
				return true, filepath.SkipDir
			}
			return true, nil
		}
	}

	return false, nil
}
