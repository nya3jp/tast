// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"go.chromium.org/tast/core/ssh"
)

// Forwarder creates a listener that forwards TCP connections to another host
// over an already-established SSH connection.
//
// A pictoral explanation:
//
//	               Local               |    SSH Host    |   Remote
//	-----------------------------------+----------------+-------------
//
// (local-to-remote)
//
//	[client] <- TCP -> [Forwarder] <- SSH -> [sshd] <- TCP -> [server]
//
// (remote-to-local)
//
//	[server] <- TCP -> [Forwarder] <- SSH -> [sshd] <- TCP -> [client]
type Forwarder = ssh.Forwarder
