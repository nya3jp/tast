// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"

	"chromiumos/tast/dut"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// Remote implements the main function for remote test bundles.
//
// The returned status code should be passed to os.Exit.
func Remote(stdin io.Reader, stdout io.Writer) int {
	args := Args{}
	cfg, status := readArgs(stdin, stdout, &args, remoteBundle)
	if status != statusSuccess || cfg == nil {
		return status
	}

	// Connect to the DUT and attach the connection to the context so tests can use it.
	if args.Target == "" {
		writeError("Target not supplied")
		return statusBadArgs
	}
	dt, err := dut.New(args.Target, args.KeyFile, args.KeyDir)
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
