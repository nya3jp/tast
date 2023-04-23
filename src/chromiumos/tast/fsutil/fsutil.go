// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fsutil implements common file operations.
package fsutil

import (
	"go.chromium.org/tast/core/fsutil"
)

// CopyFile copies the regular file at path src to dst.
// dst is atomically replaced if it already exists and inherits src's mode.
// Ownership will also be preserved if the EUID is 0.
func CopyFile(src, dst string) error {
	return fsutil.CopyFile(src, dst)
}

// MoveFile moves the file at src to dst.
// The source and destination paths may be on different filesystems.
// The mode will be preserved, and ownership will also be preserved if possible
// (i.e. if called with an EUID of 0 or if moving the file within a filesystem).
func MoveFile(src, dst string) error {
	return fsutil.MoveFile(src, dst)
}

// CopyDir copies a directory from srcDir to dstDir. Target dir must not exist.
// The mode is preserved. The owner is also preserved if the EUID is 0.
func CopyDir(srcDir, dstDir string) error {
	return fsutil.CopyDir(srcDir, dstDir)
}
