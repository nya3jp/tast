// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"chromiumos/tast/host"
)

// Forwarder creates a local listener that forwards TCP connections to a remote host
// over an already-established SSH connection.
//
// A pictoral explanation:
//
//                 Local               |    SSH Host    |   Remote
//  -----------------------------------+----------------+-------------
//  [client] <- TCP -> [Forwarder] <- SSH -> [sshd] <- TCP -> [server]
type Forwarder = host.Forwarder
