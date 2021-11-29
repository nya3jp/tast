// Copyright 2017 The Chromium OS Authors. All rights reserved.
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

	"chromiumos/tast/internal/crosbundle"
	"chromiumos/tast/internal/runner"
)

func main() {
	scfg := runner.StaticConfig{
		Type:                    runner.LocalRunner,
		KillStaleRunners:        true,
		EnableSyslog:            true,
		GetDUTInfo:              crosbundle.GetDUTInfo,
		GetSysInfoState:         crosbundle.GetSysInfoState,
		CollectSysInfo:          crosbundle.CollectSysInfo,
		PrivateBundlesStampPath: "/usr/local/share/tast/.private-bundles-downloaded",
		DeprecatedDirectRunDefaults: runner.DeprecatedDirectRunConfig{
			BundleGlob: "/usr/local/libexec/tast/bundles/local/*",
			DataDir:    "/usr/local/share/tast/data",
			TempDir:    "/usr/local/tmp/tast/run_tmp",
		},
	}
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &scfg))
}
