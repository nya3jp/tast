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
	if _, err := io.Copy(df, sf); err != nil {
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

// CopyDir copies a directory from srcDir to dstDir. Target dir must not exist.
// The mode is preserved. The owner is also preserved if the EUID is 0.
func CopyDir(srcDir, dstDir string) error {
	// Create target dir or return error if already present.
	srcStat, err := os.Stat(srcDir)
	if err != nil {
		return errors.Errorf("failed to stat %s", srcDir)
	}
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dstDir, srcStat.Mode()); err != nil {
			return errors.Wrapf(err, "failed to create %s", dstDir)
		}
	} else {
		return errors.Errorf("target dir %s exists", dstDir)
	}

	// Attempt to remove target dir if copying failed.
	success := false
	defer func() {
		if !success {
			os.RemoveAll(dstDir)
		}
	}()

	// Preserve owner attributes.
	if os.Geteuid() == 0 {
		st := srcStat.Sys().(*syscall.Stat_t)
		if err := os.Chown(dstDir, int(st.Uid), int(st.Gid)); err != nil {
			return errors.Wrapf(err, "failed to change owner of dir %s", dstDir)
		}
	}

	// Copy dir content.
	entries, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return errors.Wrapf(err, "failed to read dir %s", srcDir)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		stat, err := os.Stat(srcPath)
		if err != nil {
			return errors.Errorf("failed to stat %s", srcPath)
		}
		switch stat.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CopyDir(srcPath, dstPath); err != nil {
				return errors.Wrapf(err, "failed to copy dir %s", srcPath)
			}
		case os.ModeSymlink:
			if link, err := os.Readlink(srcPath); err != nil {
				return errors.Wrapf(err, "failed to read symlink %s", srcPath)
			} else if err := os.Symlink(link, dstPath); err != nil {
				return errors.Wrapf(err, "failed to create symlink %s", dstPath)
			}
		default:
			if err := CopyFile(srcPath, dstPath); err != nil {
				return errors.Wrapf(err, "failed to copy file %s", srcPath)
			}
		}
	}

	success = true
	return nil
}
