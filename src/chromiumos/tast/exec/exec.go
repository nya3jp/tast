// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package exec is common code used by both ssh and testexec for command execution.
package exec

// RunOption is enum of options which can be passed to Run, Output,
// CombinedOutput and Wait to control precise behavior of them.
type RunOption int

// DumpLogOnError is an option to dump logs if the executed command fails
// (i.e., exited with non-zero status code).
const DumpLogOnError RunOption = iota
