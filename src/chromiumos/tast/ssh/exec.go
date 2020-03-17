// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"chromiumos/tast/host"
)

// Cmd represents an external command being prepared or run on a remote host.
//
// This type implements almost similar interface to os/exec, but there are
// several notable differences.
//
// Command does not accept context.Context, but Cmd's methods do. This is to
// support existing use cases where we want to use different deadlines for
// different operations (e.g. Start and Wait) on the same command execution.
type Cmd = host.Cmd
