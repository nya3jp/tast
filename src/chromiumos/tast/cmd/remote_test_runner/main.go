// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the remote_test_runner executable.
//
// remote_test_runner is executed directly by the tast command.
// It runs test bundles and reports the results back to tast.
package main

import (
	"log"
	"os"

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/runner"
)

func main() {
	args := runner.Args{
		RunTests: &runner.RunTestsArgs{
			BundleGlob: "/usr/libexec/tast/bundles/remote/*", // default glob matching test bundles
			BundleArgs: bundle.RunTestsArgs{
				DataDir: "/usr/share/tast/data", // default dir containing test data
			},
		},
	}
	rpcV2 := len(os.Args) > 1 && os.Args[1] == "-rpcv2"

	cfg := runner.Config{
		Type:             runner.RemoteRunner,
		KillStaleRunners: rpcV2,
	}

	// TODO: merge logic with local_test_runner.go
	// TODO: use flag
	if rpcV2 {
		log.Println("remote runner: using RPC V2")
		os.Exit(runner.RunV2(os.Stdin, os.Stdout, &args, &cfg))
		return
	}
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &args, &cfg))
}
