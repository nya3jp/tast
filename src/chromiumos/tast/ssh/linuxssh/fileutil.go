// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxssh provides Linux specific operations conducted via SSH
// TODO(oka): now that this file is not used from framework, simplify the code.
package linuxssh

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/ssh"
)

// SymlinkPolicy describes how symbolic links should be handled by PutFiles.
type SymlinkPolicy = linuxssh.SymlinkPolicy

const (
	// PreserveSymlinks indicates that symlinks should be preserved during the copy.
	PreserveSymlinks = linuxssh.PreserveSymlinks
	// DereferenceSymlinks indicates that symlinks should be dereferenced and turned into normal files.
	DereferenceSymlinks = linuxssh.DereferenceSymlinks
)

// GetFile copies a file or directory from the host to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
func GetFile(ctx context.Context, s *ssh.Conn, src, dst string, symlinkPolicy SymlinkPolicy) error {
	return linuxssh.GetFile(ctx, s, src, dst, symlinkPolicy)
}

// PutFiles copies files on the local machine to the host. files describes
// a mapping from a local file path to a remote file path. For example, the call:
//
//	PutFiles(ctx, conn, map[string]string{"/src/from": "/dst/to"})
//
// will copy the local file or directory /src/from to /dst/to on the remote host.
// Local file paths can be absolute or relative. Remote file paths must be absolute.
// SHA1 hashes of remote files are checked in advance to send updated files only.
// bytes is the amount of data sent over the wire (possibly after compression).
func PutFiles(ctx context.Context, s *ssh.Conn, files map[string]string,
	symlinkPolicy SymlinkPolicy) (bytes int64, err error) {
	return linuxssh.PutFiles(ctx, s, files, symlinkPolicy)
}

// ReadFile reads the file on the path and returns the contents.
func ReadFile(ctx context.Context, conn *ssh.Conn, path string) ([]byte, error) {
	return conn.CommandContext(ctx, "cat", path).Output(ssh.DumpLogOnError)
}

// WriteFile writes data to the file on the path. If the file does not exist,
// WriteFile creates it with permissions perm; otherwise WriteFile truncates it
// before writing, without changing permissions.
// Unlike ioutil.WriteFile, it doesn't apply umask on perm.
func WriteFile(ctx context.Context, conn *ssh.Conn, path string, data []byte, perm os.FileMode) error {
	cmd := conn.CommandContext(ctx, "sh", "-c", `test -e "$0"; r=$?; cat > "$0"; if [ $r = 1 ]; then chmod "$1" "$0"; fi`, path, fmt.Sprintf("%o", perm&os.ModePerm))
	cmd.Stdin = bytes.NewBuffer(data)
	return cmd.Run(ssh.DumpLogOnError)
}
