// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/timing"
)

func runTests(ctx context.Context, cfg *Config) ([]TestResult, error) {
	if err := getSoftwareFeatures(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}
	if err := getInitialSysInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}
	cfg.startedRun = true

	var results []TestResult
	if cfg.runLocal {
		lres, err := runLocalTests(ctx, cfg)
		if err != nil {
			// TODO(derat): While test runners are always supposed to report success even if tests fail,
			// it'd probably be better to run both types here even if one fails.
			return nil, err
		}
		results = append(results, lres...)
	}

	// Turn down the ephemeral devserver before running remote tests. Some remote tests
	// in the meta category run the tast command which starts yet another ephemeral devserver
	// and reverse forwarding port can conflict.
	closeEphemeralDevserver(ctx, cfg)

	// Run remote tests and merge the results.
	if !cfg.runRemote {
		return results, nil
	}

	start := time.Now()
	rres, err := runRemoteTests(ctx, cfg)
	cfg.Logger.Logf("Ran %v remote test(s) in %v", len(results), time.Now().Sub(start).Round(time.Millisecond))
	results = append(results, rres...)
	return results, err
}

// runTestsOnce synchronously runs test_runner to completion.
// args.FillDeprecated() is called first to backfill any deprecated fields for old runners.
func runTestsOnce(ctx context.Context, cfg *Config, r cmd, args *runner.Args, crf copyAndRemoveFunc, df diagnoseRunErrorFunc) (
	results []TestResult, unstarted []string, rerr error) {
	ctx, st := timing.Start(ctx, "run_tests_once")
	defer st.End()

	// Set up stdin.
	args.FillDeprecated()
	stdin, err := json.Marshal(args)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %v", err)
	}
	r.SetStdin(bytes.NewBuffer(stdin))

	// Set up stdout and stderr pipes.
	stdout, err := r.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open stdout pipe: %v", err)
	}
	stderr, err := r.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open stderr pipe: %v", err)
	}

	if err := r.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start local_test_runner: %v", err)
	}

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(stderr)
	defer func() {
		// Overwrites the rerr here if the Wait() failed to provide more useful error message.
		if err := r.Wait(); err != nil {
			rerr = stderrReader.appendToError(err, stderrTimeout)
		}
	}()

	return readTestOutput(ctx, cfg, stdout, crf, df)
}
