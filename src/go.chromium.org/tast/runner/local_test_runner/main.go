// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the local_test_runner executable.
//
// local_test_runner is executed on-device by the tast command.
// It runs test bundles and reports the results back to tast.
// It is also used to query additional information about the DUT
// such as logs, crashes, and supported software features.
package main

import (
	"os"

	"go.chromium.org/tast/core/runner"
)

func main() {
	os.Exit(runner.RunLocal())
}
