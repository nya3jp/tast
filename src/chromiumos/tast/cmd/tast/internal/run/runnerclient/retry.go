// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"reflect"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/errors"
)

// runTestsFunc is a function to run local/remote tests matched with patterns.
// When it returns prematurely before running all requested tests, it may return
// a list of unstarted test names. If the function cannot compute the list,
// it should return nil as unstarted. Note that nil slice and non-nil empty
// slice are distinguished in this case; non-nil empty slice is considered that
// there is no remaining test.
type runTestsFunc func(ctx context.Context, patterns []string) (results []*resultsjson.Result, unstarted []string, err error)

// beforeRetryFunc is a function to recover from premature runner exits.
// If it is okay to proceed to retry, return true. Otherwise no further retry
// is attempted. If the function encounters any error, it should write it to
// logs.
type beforeRetryFunc func(ctx context.Context) bool

// runTestsWithRetry runs local/remote tests in a loop. If cfg.ContinueAfterFailure
// is true and runTests returns non-empty unstarted test names, it calls recover
// followed by runTests again to restart testing.
func runTestsWithRetry(ctx context.Context, cfg *config.Config, patterns []string, runTests runTestsFunc, beforeRetry beforeRetryFunc) ([]*resultsjson.Result, error) {
	var allResults []*resultsjson.Result
	for {
		results, unstarted, rerr := runTests(ctx, patterns)
		allResults = append(allResults, results...)
		if rerr == nil {
			break
		}
		cfg.Logger.Logf("Test runner failed: %v", rerr)
		// If test is terminated due to any reason such as reaching the maximum number failures,
		// we should not attempt to run tests again.
		// If we do not have this check here, local test will fail in beforeRetry when it tries
		// to contact the DUT and local test will fail when it rececive first heartbeat.
		// Therefore, it will not affect the overall result even without this check.
		// However, we will print out "Trying to run .. remaining test(s)" in log file after terminate.
		// This behavior will confuse users when they read the log file.
		// We can also use the check ctx.Err() != nil instead, but it will affect the current
		// behavior of handling timeout. Currently, runTestsWithRetry will continue to run
		// tests after timeout but it will get error in beforeRetry.
		if errors.Is(rerr, ErrTerminate) {
			return allResults, rerr
		}
		// If runTests didn't provide a list of remaining tests, give up.
		if unstarted == nil {
			return allResults, rerr
		}
		// If we know that there are no more tests left to execute, report the overall run as having succeeded.
		// The test that was in progress when the run failed will be reported as having failed.
		if len(unstarted) == 0 {
			break
		}
		// If we don't want to try again, or we'd just be doing the same thing that we did last time, give up.
		if !cfg.ContinueAfterFailure || reflect.DeepEqual(patterns, unstarted) {
			return allResults, rerr
		}

		cfg.Logger.Logf("Trying to run %v remaining test(s)", len(unstarted))

		if !beforeRetry(ctx) {
			return allResults, rerr
		}
		patterns = unstarted
	}

	return allResults, nil
}
