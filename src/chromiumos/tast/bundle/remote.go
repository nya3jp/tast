// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"errors"
	"fmt"
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
func Remote(stdin io.Reader, stdout, stderr io.Writer) int {
	args := Args{}
	cfg := runConfig{
		runSetupFunc: func(ctx context.Context) (context.Context, error) {
			// Connect to the DUT and attach the connection to the context so tests can use it.
			if args.Target == "" {
				return ctx, errors.New("target not supplied")
			}
			dt, err := dut.New(args.Target, args.KeyFile, args.KeyDir)
			if err != nil {
				return ctx, fmt.Errorf("failed to create connection: %v", err.Error())
			}
			ctx = dut.NewContext(ctx, dt)
			if err = dt.Connect(ctx); err != nil {
				return ctx, fmt.Errorf("failed to connect to DUT: %v", err.Error())
			}
			return ctx, nil
		},
		runCleanupFunc: func(ctx context.Context) error {
			dt, ok := dut.FromContext(ctx)
			if !ok {
				return errors.New("failed to get DUT from context")
			}
			return dt.Close(ctx)
		},
		testSetupFunc: func(ctx context.Context) error {
			// Reconnect between tests if needed.
			if dt, ok := dut.FromContext(ctx); !ok {
				return errors.New("failed to get DUT from context")
			} else if !dt.Connected(ctx) {
				return dt.Connect(ctx)
			}
			return nil
		},
		defaultTestTimeout: remoteTestTimeout,
	}

	return run(context.Background(), stdin, stdout, stderr, &args, &cfg, remoteBundle)
}
