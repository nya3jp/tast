// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the remote_test_runner executable.
//
// remote_test_runner is executed directly by the tast command.
// It runs test bundles and reports the results back to tast.
package main

import (
	"flag"
	"os"

	"chromiumos/tast/runner"
)

const (
	defaultBundleGlob = "/usr/libexec/tast/bundles/remote/*" // default glob matching test bundles
	defaultDataDir    = "/usr/share/tast/data/remote"        // default dir containing test data
)

func main() {
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	target := flags.String("target", "", "DUT connection spec as \"[<user>@]host[:<port>]\"")
	keyfile := flags.String("keyfile", "", "path to SSH private key to use for connecting to DUT")
	keydir := flags.String("keydir", "", "directory containing SSH private keys (typically $HOME/.ssh)")
	cfg, status := runner.ParseArgs(os.Stdout, os.Args[1:], defaultBundleGlob, defaultDataDir, flags)
	if status != 0 || cfg == nil {
		os.Exit(status)
	}

	cfg.ExtraFlags = []string{"-target=" + *target, "-keyfile=" + *keyfile, "-keydir=" + *keydir}
	os.Exit(runner.RunTests(cfg))
}
