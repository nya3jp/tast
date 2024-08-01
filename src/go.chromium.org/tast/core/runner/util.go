// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runner provides utilities for Tast runner executables.
package runner

import (
	"os"

	"go.chromium.org/tast/core/internal/crosbundle"
	"go.chromium.org/tast/core/internal/runner"
)

// RunLocal runs the local test runner.
func RunLocal() int {
	scfg := runner.StaticConfig{
		Type:                    runner.LocalRunner,
		KillStaleRunners:        true,
		EnableSyslog:            true,
		GetDUTInfo:              crosbundle.GetDUTInfo,
		GetSysInfoState:         crosbundle.GetSysInfoState,
		CollectSysInfo:          crosbundle.CollectSysInfo,
		BundleType:              runner.Local,
		PrivateBundlesStampPath: "/usr/local/share/tast/.private-bundles-downloaded",
		DeprecatedDirectRunDefaults: runner.DeprecatedDirectRunConfig{
			BundleGlob: "/usr/local/libexec/tast/bundles/local/*",
			DataDir:    "/usr/local/share/tast/data",
			TempDir:    "/usr/local/tmp/tast/run_tmp",
		},
	}
	return runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &scfg)
}
