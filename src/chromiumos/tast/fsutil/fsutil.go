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

	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
)

// CopyFile copies the regular file at path src to dst.
// dst is atomically replaced if it already exists and inherits src's mode.
// Ownership will also be preserved if the EUID is 0.
func CopyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "failed to open src file")
	}
	defer sf.Close()

	fi, err := sf.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to stat src file")
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("source not regular file (mode %s)", fi.Mode())
	}

	// Copy to a temp file.
	df, err := ioutil.TempFile(filepath.Dir(dst), "."+filepath.Base(dst)+".")
	if err != nil {
		return errors.Wrap(err, "failed to create tmp file")
	}
	// TODO(b:178332739): CopyFile triggers a kernel bug as copy_file_range does not
	// behave correctly on kernel >=5.4 for zero-sized sysfs files.
	if _, err := io.Copy(struct{ io.Writer }{df}, struct{ io.Reader }{sf}); err != nil {
		df.Close()
		os.Remove(df.Name())
		return errors.Wrap(err, "failed to copy data from src file to tmp file")
	}
	if err := df.Close(); err != nil {
		os.Remove(df.Name())
		return errors.Wrap(err, "failed to close tmp file")
	}

	// Finally, set the mode and ownership and rename to the requested name.
	if err := os.Chmod(df.Name(), fi.Mode()); err != nil {
		os.Remove(df.Name())
		return errors.Wrap(err, "failed to change permissions of tmp file")
	}
	if os.Geteuid() == 0 {
		st := fi.Sys().(*syscall.Stat_t)
		if err := os.Chown(df.Name(), int(st.Uid), int(st.Gid)); err != nil {
			os.Remove(df.Name())
			return errors.Wrap(err, "failed to change owner of tmp file")
		}
	}
	if err := os.Rename(df.Name(), dst); err != nil {
		os.Remove(df.Name())
		return errors.Wrap(err, "failed to rename tmp file to dst file")
	}
	return nil
}

// MoveFile moves the file at src to dst.
// The source and destination paths may be on different filesystems.
// The mode will be preserved, and ownership will also be preserved if possible
// (i.e. if called with an EUID of 0 or if moving the file within a filesystem).
func MoveFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, "failed to open src file")
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("source not regular file (mode %s)", fi.Mode())
	}

	// Try to do a rename first. This only works if we're not moving across filesystems.
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if lerr, ok := err.(*os.LinkError); !ok || lerr.Err != unix.EXDEV {
		return errors.Wrap(err, "failed to rename src to dst file")
	}

	// Otherwise, copy before deleting src.
	if err := CopyFile(src, dst); err != nil {
		return errors.Wrap(err, "failed to copy src to dst file")
	}
	return os.Remove(src)
}
