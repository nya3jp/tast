// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package host implements communication with a remote host.
package host

import (
	"context"
	"io"
	"strings"
	"time"
)

// InputMode describes how stdin should be handled when running a remote command.
// Commands may block if stdin is never closed.
type InputMode int

// OutputMode describes how stdout and stderr should be handled when running a remote command.
// Commands may block if output is not consumed.
type OutputMode int

const (
	// OpenStdin indicates that stdin should be copied to the remote command.
	OpenStdin InputMode = iota
	// CloseStdin indicates that stdin should be closed.
	CloseStdin

	// StdoutAndStderr indicates that stdout and stderr should be merged together.
	StdoutAndStderr OutputMode = iota
	// Stdout indicates that only stdout should be returned (i.e. stderr should be closed).
	StdoutOnly
	// Stderr indicates that only stderr should be returned (i.e. stdout should be closed).
	StderrOnly
	// NoOutput indicates that both stdout and stderr should be closed.
	NoOutput

	// Bogus hash value used for local directories.
	localDirHash = "dir"
)

// Host represents a connection to another computer.
type Host interface {
	// Close closes the underlying connection to the host.
	Close(ctx context.Context) error

	// GetFile copies a file or directory from the host to the local machine.
	// dst is the full destination name for the file or directory being copied, not
	// a destination directory into which it will be copied. dst must not already exist.
	GetFile(ctx context.Context, src, dst string) error

	// PutFile copies a file or directory from the local machine to the host.
	// dst is the full destination name for the file or directory being copied, not
	// a destination directory into which it will be copied. If dst already exists, it
	// will be overwritten. bytes is the amount of data sent over the wire (possibly
	// after compression).
	PutFile(ctx context.Context, src, dst string) (bytes int64, err error)

	// PutTree copies all relative paths in files from srcDir on the local machine
	// to dstDir on the host. For example, the call:
	//
	//	PutTree("/src", "/dst", []string{"a", "dir/b"})
	//
	// will result in the local file or directory /src/a being copied to /dst/a on
	// the remote host and /src/dir/b being copied to /dst/dir/b. Existing files will be
	// overwritten. bytes is the amount of data sent over the wire (possibly after compression).
	PutTree(ctx context.Context, srcDir, dstDir string, files []string) (bytes int64, err error)

	// Run runs cmd synchronously on the host and returns its output. stdout and stderr are combined.
	Run(ctx context.Context, cmd string) ([]byte, error)

	// Start runs cmd asynchronously on the host and returns a handle that can be used to write input,
	// read output, and wait for completion.
	Start(ctx context.Context, cmd string, input InputMode, output OutputMode) (CommandHandle, error)

	// Ping checks that the connection to the host is still active, blocking until a
	// response has been received. An error is returned if the connection is inactive or
	// if timeout or ctx's deadline are exceeded.
	Ping(ctx context.Context, timeout time.Duration) error
}

// CommandHandle represents an ongoing command running on a Host.
type CommandHandle interface {
	// Close closes the session in which the command is running.
	Close(ctx context.Context) error

	// Stderr returns a pipe connected to the command's stderr or nil if the OutputMode didn't include stderr.
	Stderr() io.Reader

	// Stdin returns a pipe connected to the command's stdin or nil if the InputMode was not OpenStdin.
	Stdin() io.WriteCloser

	// Stdout returns a pipe connected to the command's stdout or nil if the OutputMode didn't include stdin.
	Stdout() io.Reader

	// Wait waits until the command finishes running.
	Wait(ctx context.Context) error
}

// QuoteShellArg returns a single-quoted copy of s that can be inserted into command lines interpreted by sh.
func QuoteShellArg(s string) string {
	s = strings.Replace(s, "'", "'\"'\"'", -1)
	return "'" + s + "'"
}
