// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxssh provides Linux specific operations conducted via SSH
package linuxssh

import (
	"context"

	"chromiumos/tast/internal/host"
	"chromiumos/tast/ssh"
)

// GetFile copies a file or directory from the host to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
func GetFile(ctx context.Context, c *ssh.Conn, src, dst string) error {
	return c.DeprecatedGetFile(ctx, src, dst)
}

// SymlinkPolicy describes how symbolic links should be handled by PutFiles.
type SymlinkPolicy = host.DeprecatedSymlinkPolicy

const (
	// PreserveSymlinks indicates that symlinks should be preserved during the copy.
	PreserveSymlinks SymlinkPolicy = iota
	// DereferenceSymlinks indicates that symlinks should be dereferenced and turned into normal files.
	DereferenceSymlinks
)

// PutFiles copies files on the local machine to the host. files describes
// a mapping from a local file path to a remote file path. For example, the call:
//
//	PutFiles(ctx, map[string]string{"/src/from": "/dst/to"})
//
// will copy the local file or directory /src/from to /dst/to on the remote host.
// Local file paths can be absolute or relative. Remote file paths must be absolute.
// SHA1 hashes of remote files are checked in advance to send updated files only.
// bytes is the amount of data sent over the wire (possibly after compression).
func PutFiles(ctx context.Context, c *ssh.Conn, files map[string]string,
	symlinkPolicy SymlinkPolicy) (bytes int64, err error) {
	return c.DeprecatedPutFiles(ctx, files, symlinkPolicy)
}

// DeleteTree deletes all relative paths in files from baseDir on the host.
// If a specified file is a directory, all files under it are recursively deleted.
// Non-existent files are ignored.
func DeleteTree(ctx context.Context, c *ssh.Conn, baseDir string, files []string) error {
	return c.DeprecatedDeleteTree(ctx, baseDir, files)
}
