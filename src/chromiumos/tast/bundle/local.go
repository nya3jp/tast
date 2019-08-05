// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"

	"chromiumos/tast/faillog"
	"chromiumos/tast/testing"
)

const (
	localTestTimeout = 2 * time.Minute              // default max runtime for each test
	localTestDataDir = "/usr/local/share/tast/data" // default dir for test data
)

// LocalDelegate injects several functions as a part of local test bundle framework implementation.
type LocalDelegate struct {
	// Ready can be passed to the Local function to wait for the DUT to be ready for tests to run.
	// Informational messages can be passed to log and should be written at least once per minute to
	// let the tast process (and the user) know the reason for the delay.
	// If an error is returned, none of the bundle's tests will run.
	Ready func(ctx context.Context, log func(string)) error

	// Faillog is called on each test failure. It is expected to save debugging info.
	Faillog func(ctx context.Context, s *testing.State)
}

// Local implements the main function for local test bundles.
//
// clArgs should typically be os.Args[1:].
// If ready is non-nil, it will be executed before any tests from this bundle are run to ensure
// that the DUT is ready for testing. This can be used to wait for all important system services
// to be running on a newly-booted DUT, for instance.
// The returned status code should be passed to os.Exit.
func Local(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, delegate LocalDelegate) int {
	args := Args{RunTests: &RunTestsArgs{DataDir: localTestDataDir}}
	cfg := runConfig{
		postTestFunc: func(ctx context.Context, s *testing.State) {
			if !s.HasError() {
				return
			}
			if delegate.Faillog != nil {
				delegate.Faillog(ctx, s)
			} else {
				// TODO(crbug.com/984390): Remove this after faillog migration to
				// tast-tests repository.
				faillog.Save(ctx, s)
			}
		},
		defaultTestTimeout: localTestTimeout,
	}
	if delegate.Ready != nil {
		cfg.preRunFunc = func(ctx context.Context, lf logFunc) (context.Context, error) {
			if !args.RunTests.WaitUntilReady {
				return ctx, nil
			}
			lf("Waiting for DUT to be ready for testing")
			return ctx, delegate.Ready(ctx, lf)
		}
	}
	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, localBundle)
}
