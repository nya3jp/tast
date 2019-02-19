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
	"chromiumos/tast/testing"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// Remote implements the main function for remote test bundles.
//
// clArgs should typically be os.Args[1:].
// The returned status code should be passed to os.Exit.
func Remote(clArgs []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args := Args{}
	cfg := runConfig{
		preRunFunc: func(ctx context.Context, lf logFunc) (context.Context, error) {
			// Connect to the DUT and attach the connection to the context so tests can use it.
			if args.Target == "" {
				return ctx, errors.New("target not supplied")
			}
			lf("Connecting to DUT")
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
		postRunFunc: func(ctx context.Context, lf logFunc) error {
			dt, ok := dut.FromContext(ctx)
			if !ok {
				return errors.New("failed to get DUT from context")
			}
			lf("Disconnecting from DUT")
			return dt.Close(ctx)
		},
		preTestFunc: func(ctx context.Context, s *testing.State) {
			// Reconnect between tests if needed.
			if dt, ok := dut.FromContext(ctx); !ok {
				s.Fatal("Failed to get DUT from context")
			} else if !dt.Connected(ctx) {
				s.Log("Reconnecting to DUT")
				if err := dt.Connect(ctx); err != nil {
					s.Fatal("Failed to reconnect to DUT: ", err)
				}
			}
		},
		defaultTestTimeout: remoteTestTimeout,
	}

	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, remoteBundle)
}
