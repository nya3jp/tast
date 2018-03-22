// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the local_test_runner executable.
//
// local_test_runner is executed on-device by the tast command.
// It runs test bundles and reports the results back to tast.
package main

import (
	"os"

	"chromiumos/tast/crash"
	"chromiumos/tast/runner"
)

const (
	defaultBundleGlob = "/usr/local/libexec/tast/bundles/local/*" // default glob matching test bundles
	defaultDataDir    = "/usr/local/share/tast/data/local"        // default dir containing test data
	systemLogDir      = "/var/log"                                // directory where system logs are located
)

func main() {
	args := runner.Args{
		BundleGlob:      defaultBundleGlob,
		DataDir:         defaultDataDir,
		SystemLogDir:    systemLogDir,
		SystemCrashDirs: []string{crash.DefaultCrashDir, crash.ChromeCrashDir},
	}
	cfg, status := runner.ParseArgs(os.Args[1:], os.Stdin, os.Stdout, &args, runner.LocalRunner)
	if status != 0 || cfg == nil {
		os.Exit(status)
	}
	os.Exit(runner.RunTests(cfg))
}
