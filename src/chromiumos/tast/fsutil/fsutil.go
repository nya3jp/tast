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

	"chromiumos/tast/errors"
)

// HACK HACK HACK: Functions from go 1.13
// https://github.com/golang/go/blame/release-branch.go1.13/src/io/io.go
func copyBuffer113(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}
	if buf == nil {
		size := 32 * 1024
		if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, size)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func Copy113(dst io.Writer, src io.Reader) (written int64, err error) {
	return copyBuffer113(dst, src, nil)
}

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
	if _, err := Copy113(df, sf); err != nil {
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
	} else if lerr, ok := err.(*os.LinkError); !ok || lerr.Err != syscall.EXDEV {
		return errors.Wrap(err, "failed to rename src to dst file")
	}

	// Otherwise, copy before deleting src.
	if err := CopyFile(src, dst); err != nil {
		return errors.Wrap(err, "failed to copy src to dst file")
	}
	return os.Remove(src)
}
