// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"

	"chromiumos/tast/dut"
	"chromiumos/tast/internal/testing"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// RemoteDelegate injects functions as a part of remote test bundle framework implementation.
type RemoteDelegate struct {
	// TestHook is called before each test run if it is not nil. The returned closure is executed
	// after the test if not nil.
	TestHook func(ctx context.Context, s *testing.TestHookState) func(ctx context.Context, s *testing.TestHookState)

	// BeforeReboot is called before every rebooting if it is not nil.
	BeforeReboot func(ctx context.Context, d *dut.DUT) error
}

// Remote implements the main function for remote test bundles.
//
// Main function of remote test bundles should call RemoteDefault instead.
func Remote(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, delegate RemoteDelegate) int {
	args := Args{}
	cfg := runConfig{
		defaultTestTimeout: remoteTestTimeout,
		testHook:           delegate.TestHook,
		beforeReboot:       delegate.BeforeReboot,
	}
	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, remoteBundle)
}
