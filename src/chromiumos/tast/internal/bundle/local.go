// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
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

	// RunHook is called at the beginning of a bundle execution if it is not nil.
	// The returned closure is executed at the end if it is not nil.
	RunHook func(ctx context.Context) (func(ctx context.Context) error, error)

	// TestHook is called before each test run if it is not nil. The returned closure is executed
	// after the test if not nil.
	TestHook func(ctx context.Context, s *testing.TestHookState) func(ctx context.Context, s *testing.TestHookState)
}

// Local implements the main function for local test bundles.
//
// Main function of local test bundles should call LocalDefault instead.
func Local(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, delegate LocalDelegate) int {
	args := Args{RunTests: &RunTestsArgs{DataDir: localTestDataDir}}
	cfg := runConfig{
		defaultTestTimeout: localTestTimeout,
		testHook:           delegate.TestHook,
	}
	cfg.runHook = func(ctx context.Context) (func(context.Context) error, error) {
		if delegate.Ready != nil && args.RunTests.WaitUntilReady {
			if err := delegate.Ready(ctx); err != nil {
				return nil, err
			}
		}
		if delegate.RunHook == nil {
			return nil, nil
		}
		return delegate.RunHook(ctx)
	}
	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, localBundle)
}
