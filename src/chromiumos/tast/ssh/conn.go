// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"context"

	"chromiumos/tast/internal/host"
)

// Conn represents an SSH connection to another computer.
type Conn = host.SSH

// Options contains options used when connecting to an SSH server.
type Options = host.SSHOptions

// New establishes an SSH connection to the host described in o.
// Callers are responsible to call Conn.Close after using it.
func New(ctx context.Context, o *Options) (*Conn, error) {
	return host.NewSSH(ctx, o)
}

// ParseTarget parses target (of the form "[<user>@]host[:<port>]") and fills
// the User, Hostname, and Port fields in o, using reasonable defaults for unspecified values.
func ParseTarget(target string, o *Options) error {
	return host.ParseSSHTarget(target, o)
}

/*
TODO: Move following methods from host package:
- Close
- ListenTCP
- NewForwarder
- Ping
*/
