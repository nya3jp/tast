// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"context"

	"go.chromium.org/tast/core/ssh"
)

// Conn represents an SSH connection to another computer.
type Conn = ssh.Conn

// Options contains options used when connecting to an SSH server.
type Options = ssh.Options

// ParseTarget parses target (of the form "[<user>@]host[:<port>]") and fills
// the User, Hostname, and Port fields in o, using reasonable defaults for unspecified values.
func ParseTarget(target string, o *Options) error {
	return ssh.ParseTarget(target, o)
}

// New establishes an SSH connection to the host described in o.
// Callers are responsible to call Conn.Close after using it.
func New(ctx context.Context, o *Options) (*Conn, error) {
	return ssh.New(ctx, o)
}
