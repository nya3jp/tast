// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the remote_test_runner executable.
//
// remote_test_runner is executed directly by the tast command.
// It runs test bundles and reports the results back to tast.
package main

import (
	"os"

	"chromiumos/tast/runner"
)

const (
	defaultBundleGlob = "/usr/libexec/tast/bundles/remote/*" // default glob matching test bundles
	defaultDataDir    = "/usr/share/tast/data/remote"        // default dir containing test data
)

func main() {
	args := runner.Args{
		BundleGlob: defaultBundleGlob,
		DataDir:    defaultDataDir,
	}
	cfg, status := runner.ParseArgs(os.Args[1:], os.Stdin, os.Stdout, &args, runner.RemoteRunner)
	if status != 0 || cfg == nil {
		os.Exit(status)
	}
	cfg.ExtraFlags = []string{
		"-target=" + args.Target,
		"-keyfile=" + args.KeyFile,
		"-keydir=" + args.KeyDir,
	}
	os.Exit(runner.RunTests(cfg))
}
