// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package exec is common code used by both ssh and testexec for command execution.
package exec

import (
	"context"
)

// RunOption is enum of options which can be passed to Run, Output,
// CombinedOutput and Wait to control precise behavior of them.
type RunOption int

// DumpLogOnError is an option to dump logs if the executed command fails
// (i.e., exited with non-zero status code).
const DumpLogOnError RunOption = iota

// Cmd represents an external command being prepared or run.
//
// This struct embeds Cmd in os/exec.
//
// Callers may wish to use shutil.EscapeSlice when logging Args.
type Cmd interface {
	// Run runs an external command and waits for its completion.
	//
	// See os/exec package for details.
	Run(opts ...RunOption) error
	// Output runs an external command, waits for its completion and returns
	// stdout output of the command.
	//
	// See os/exec package for details.
	Output(opts ...RunOption) ([]byte, error)
	// CombinedOutput runs an external command, waits for its completion and
	// returns stdout/stderr output of the command.
	//
	// See os/exec package for details.
	CombinedOutput(opts ...RunOption) ([]byte, error)
	// Start starts an external command.
	//
	// See os/exec package for details.
	Start() error
	// Wait waits for the process to finish and releases all associated resources.
	//
	// See os/exec package for details.
	Wait(opts ...RunOption) error
	// DumpLog logs details of the executed external command, including uncaptured output.
	//
	// This is a new method that does not exist in os/exec.
	//
	// Call this function when the test is failing due to unexpected external command result.
	// You should not call this function for every external command invocation to avoid
	// spamming logs.
	//
	// This function must be called after Wait.
	DumpLog(ctx context.Context) error
}
