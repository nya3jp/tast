// Copyright 2023 The ChromiumOS Authors
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
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/linuxssh"
	"go.chromium.org/tast/core/ssh"
)

// SymlinkPolicy describes how symbolic links should be handled by PutFiles.
type SymlinkPolicy = linuxssh.SymlinkPolicy

const (
	// PreserveSymlinks indicates that symlinks should be preserved during the copy.
	PreserveSymlinks = linuxssh.PreserveSymlinks
	// DereferenceSymlinks indicates that symlinks should be dereferenced and turned into normal files.
	DereferenceSymlinks = linuxssh.DereferenceSymlinks
)

// WordCountInfo describes the result from the unix command wc.
type WordCountInfo struct {
	Lines int64
	Words int64
	Bytes int64
}

// GetFile copies a file or directory from the host to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
func GetFile(ctx context.Context, s *ssh.Conn, src, dst string, symlinkPolicy SymlinkPolicy) error {
	return linuxssh.GetFile(ctx, s, src, dst, symlinkPolicy)
}

// RemoteFileDelta describes the result from the function NewRemoteFileDelta.
type RemoteFileDelta struct {
	src       string
	dst       string
	maxsize   int64
	startline int64
}

// NewRemoteFileDelta gets the starting line of from DUT and then save the
// source file, destintion file and the starting line to RemoteFileDelta which
// will be returned to the caller.
func NewRemoteFileDelta(ctx context.Context, conn *ssh.Conn, src, dst string, maxSize int64) (*RemoteFileDelta, error) {
	wordCountInfo, err := WordCount(ctx, conn, src)
	if err != nil {
		return nil, fmt.Errorf("failed the get line count: %v", err)
	}
	rtd := RemoteFileDelta{
		src:       src,
		dst:       dst,
		maxsize:   maxSize,
		startline: wordCountInfo.Lines + 1,
	}

	return &rtd, nil
}

// Save calls the GetFileTail with struct RemoteFileDelta. The range between
// beginning of the file and rtd.startline will be truncated.
func (rtd *RemoteFileDelta) Save(ctx context.Context, conn *ssh.Conn) error {
	return GetFileTail(ctx, conn, rtd.src, rtd.dst, rtd.startline, rtd.maxsize)
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

// GetFileTail reads the file on the path and returns the file truncate by command tail
// with the Max System Message Log Size and destination.
func GetFileTail(ctx context.Context, conn *ssh.Conn, src, dst string, startLine, maxSize int64) error {
	return linuxssh.GetFileTail(ctx, conn, src, dst, startLine, maxSize)
}

// WriteFile writes data to the file on the path. If the file does not exist,
// WriteFile creates it with permissions perm; otherwise WriteFile truncates it
// before writing, without changing permissions.
// Unlike os.WriteFile, it doesn't apply umask on perm.
func WriteFile(ctx context.Context, conn *ssh.Conn, path string, data []byte, perm os.FileMode) error {
	cmd := conn.CommandContext(ctx, "sh", "-c", `test -e "$0"; r=$?; cat > "$0"; if [ $r = 1 ]; then chmod "$1" "$0"; fi`, path, fmt.Sprintf("%o", perm&os.ModePerm))
	cmd.Stdin = bytes.NewBuffer(data)
	return cmd.Run(ssh.DumpLogOnError)
}

// WordCount get the line, word and byte counts of a remote text file.
func WordCount(ctx context.Context, conn *ssh.Conn, path string) (*WordCountInfo, error) {
	cmd := conn.CommandContext(ctx, "wc", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call wc for %s", path)
	}
	// Output example: "   68201  1105834 14679551 /var/log/messages".
	strList := strings.Split(string(output), " ")
	var strs []string
	for _, s := range strList {
		if s != "" {
			strs = append(strs, s)
		}
	}
	if len(strs) < 3 {
		return nil, errors.Errorf("wc information is not available for %s", path)
	}
	lc, err := strconv.ParseInt(strs[0], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse line count from string %s", string(output))
	}
	wc, err := strconv.ParseInt(strs[1], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse word count from string %s", string(output))
	}
	bc, err := strconv.ParseInt(strs[2], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse bytes count from string %s", string(output))
	}
	return &WordCountInfo{Lines: lc, Words: wc, Bytes: bc}, nil
}

// WaitUntilFileExists checks if a file exists on an interval until it either
// exists or the timeout is reached.
func WaitUntilFileExists(ctx context.Context, conn *ssh.Conn, path string, timeout, interval time.Duration) error {
	cmd := conn.CommandContext(ctx, "timeout", timeout.String(), "sh", "-c", `while [ ! -e "$0" ]; do sleep $1; done`, path, interval.String())
	return cmd.Run()
}
