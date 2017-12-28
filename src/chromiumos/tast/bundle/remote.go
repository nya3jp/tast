// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"flag"
	"os"
	"time"

	"chromiumos/tast/dut"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// Remote implements the main function for remote test bundles.
//
// args should typically be os.Args[1:]. The returned status code should be passed to os.Exit.
func Remote(args []string) int {
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	target := flags.String("target", "", "DUT connection spec as \"[<user>@]host[:<port>]\"")
	keypath := flags.String("keypath", "", "path to SSH private key to use for connecting to DUT")
	cfg, status := parseArgs(os.Stdout, args, "", flags)
	if status != statusSuccess || cfg == nil {
		return status
	}

	// Don't bother connecting to the DUT if we wouldn't run any tests.
	if len(cfg.tests) == 0 {
		return statusSuccess
	}

	// Connect to the DUT and attach the connection to the context so tests can use it.
	if *target == "" {
		writeError("-target not supplied")
		return statusBadArgs
	}
	dt, err := dut.New(*target, *keypath)
	if err != nil {
		writeError("Failed to create connection: " + err.Error())
		return statusBadArgs
	}
	ctx := dut.NewContext(context.Background(), dt)
	if err = dt.Connect(ctx); err != nil {
		writeError("Failed to connect to DUT: " + err.Error())
		return statusError
	}
	defer dt.Close(ctx)

	// Reconnect between tests if needed.
	cfg.setupFunc = func() error {
		if !dt.Connected(ctx) {
			return dt.Connect(ctx)
		}
		return nil
	}
	cfg.defaultTestTimeout = remoteTestTimeout

	return runTests(ctx, cfg)
}
