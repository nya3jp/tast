// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fsutil implements common file operations.
package fsutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

// CopyFile copies the regular file at path src to dst.
// dst is truncated if it already exists and created with mode 0666 (before umask).
// Options can be passed to override the default behavior.
// If any operations fail, the destination file will be removed.
func CopyFile(src, dst string, options ...copyOption) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	if fi, err := sf.Stat(); err != nil {
		return err
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("source not regular file (mode %s)", fi.Mode())
	}

	// Copy to a temp file.
	df, err := ioutil.TempFile(filepath.Dir(dst), "."+filepath.Base(dst)+".")
	if err != nil {
		return err
	}
	if _, err := io.Copy(df, sf); err != nil {
		df.Close()
		os.Remove(df.Name())
		return err
	}
	if err := df.Close(); err != nil {
		os.Remove(df.Name())
		return err
	}

	// Apply options.
	for _, o := range options {
		if err := o(df.Name()); err != nil {
			os.Remove(df.Name())
			return err
		}
	}

	// Finally, rename to the requested name.
	if err := os.Rename(df.Name(), dst); err != nil {
		os.Remove(df.Name())
		return err
	}
	return nil
}

// copyOption contains an operation to be performed on the dst path passed to CopyFile.
type copyOption func(fn string) error

// CopyOwner returns an option that can be passed to CopyFile to set dst's UID and GID.
// A value of -1 for either ID will result in that ID not being explicitly set.
func CopyOwner(uid, gid int) copyOption {
	return func(fn string) error {
		if uid < 0 && gid < 0 {
			return nil
		}
		return os.Chown(fn, uid, gid)
	}
}

// CopyMode returns an option that can be passed to CopyFile to set dst's mode.
// The mode's permission bits, os.ModeSetuid, os.ModeSetgid, and os.ModeSticky are used.
func CopyMode(mode os.FileMode) copyOption {
	return func(fn string) error {
		return os.Chmod(fn, mode)
	}
}

// MoveFile moves the file at src to dst.
// The source and destination paths may be on different filesystems.
// The mode will be preserved, and ownership will also be preserved if possible
// (i.e. if called with an EUID of 0 or if moving the file within a filesystem).
func MoveFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("source not regular file (mode %s)", fi.Mode())
	}

	// Try to do a rename first. This only works if we're not moving across filesystems.
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Otherwise, copy and restore metadata before deleting src.
	st := fi.Sys().(*syscall.Stat_t)
	opts := []copyOption{CopyMode(fi.Mode())}
	if os.Geteuid() == 0 {
		opts = append(opts, CopyOwner(int(st.Uid), int(st.Gid)))
	}
	if err := CopyFile(src, dst, opts...); err != nil {
		return err
	}
	return os.Remove(src)
}
