// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"os"
	"time"

	"chromiumos/tast/internal/testing"
)

const (
	localTestTimeout = 2 * time.Minute              // default max runtime for each test
	localTestDataDir = "/usr/local/share/tast/data" // default dir for test data
)

// LocalDelegate injects several functions as a part of local test bundle framework implementation.
type LocalDelegate struct {
	// Ready can be passed to the Local function to wait for the DUT to be ready for tests to run.
	// Informational messages should be written with testing.ContextLog at least once per minute to
	// let the tast process (and the user) know the reason for the delay.
	// If an error is returned, none of the bundle's tests will run.
	Ready func(ctx context.Context) error

	// PreTestRun is called before each test run. The returned closure is executed after the test if not nil.
	PreTestRun func(ctx context.Context, s *testing.State) func(ctx context.Context, s *testing.State)
}

// LocalDefault implements the main function for local test bundles.
//
// Usually the Main function of a local test bundles should just this function,
// and pass the returned status code to os.Exit.
func LocalDefault(delegate LocalDelegate) int {
	stdin, stdout, stderr := lockStdIO()
	return Local(os.Args[1:], stdin, stdout, stderr, delegate)
}

// Local implements the main function for local test bundles.
//
// Main function of local test bundles should call LocalDefault instead.
func Local(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, delegate LocalDelegate) int {
	args := Args{RunTests: &RunTestsArgs{DataDir: localTestDataDir}}
	cfg := runConfig{
		defaultTestTimeout: localTestTimeout,
	}

	cfg.preTestFunc = delegate.PreTestRun

	if delegate.Ready != nil {
		cfg.preRunFunc = func(ctx context.Context) (context.Context, error) {
			if !args.RunTests.WaitUntilReady {
				return ctx, nil
			}
			return ctx, delegate.Ready(ctx)
		}
	}
	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, localBundle)
}
