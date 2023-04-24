// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"go.chromium.org/tast/core/ssh"
)

// Cmd represents an external command being prepared or run on a remote host.
//
// This type implements the almost exactly the same interface as Cmd in os/exec.
type Cmd = ssh.Cmd

// RunOption is enum of options which can be passed to Run, Output,
// CombinedOutput and Wait to control precise behavior of them.
type RunOption = ssh.RunOption

// DumpLogOnError instructs to dump logs if the executed command fails
// (i.e., exited with non-zero status code).
const DumpLogOnError = ssh.DumpLogOnError
