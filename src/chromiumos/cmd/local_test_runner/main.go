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

	"chromiumos/tast/crash"
	"chromiumos/tast/runner"
)

func main() {
	args := runner.Args{
		BundleGlob:      "/usr/local/libexec/tast/bundles/local/*",
		DataDir:         "/usr/local/share/tast/data/local",
		SystemLogDir:    "/var/log",
		SystemCrashDirs: []string{crash.DefaultCrashDir, crash.ChromeCrashDir},
		// The tast-use-flags package attempts to install this file to /etc,
		// but it gets diverted to /usr/local since it's installed for test images.
		USEFlagsFile: "/usr/local/etc/tast_use_flags.txt",
		SoftwareFeatureDefinitions: map[string]string{
			// This list is documented at docs/test_dependencies.md.
			"android":      "arc",
			"chrome":       "!chromeless_tty",
			"chrome_login": "!chromeless_tty && !rialto",
			"vm_host":      "kvm_host",
		},
	}
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &args, runner.LocalRunner))
}
