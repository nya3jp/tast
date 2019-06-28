// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"
	"fmt"

	"github.com/google/subcommands"
)

// Status describes the result of a Run call.
type Status struct {
	// ExitCode contains the exit code that should be used by the tast process.
	ExitCode subcommands.ExitStatus
	// ErrorMsg describes the reason why the run failed.
	ErrorMsg string
	// FailedBeforeRun is true if a failure occurred before trying to run tests,
	// e.g. while compiling tests. If so, the caller shouldn't write a results dir.
	FailedBeforeRun bool
}

// successStatus describes a successful run.
var successStatus = Status{}

// errorStatusf returns a Status describing a failing run. format and args are combined to produce the error
// message, which is both logged to cfg.Logger and included in the returned status.
func errorStatusf(cfg *Config, code subcommands.ExitStatus, format string, args ...interface{}) Status {
	msg := fmt.Sprintf(format, args...)
	cfg.Logger.Log(msg)
	return Status{ExitCode: code, ErrorMsg: msg}
}

// Run executes or lists tests per cfg and returns the results.
// Messages are logged using cfg.Logger as the run progresses.
// If an error is encountered, status.ErrorMsg will be logged to cfg.Logger before returning,
// but the caller may wish to log it again later to increase its prominence if additional messages are logged.
func Run(ctx context.Context, cfg *Config) (status Status, results []TestResult) {
	defer func() {
		// If we didn't get to the point where we started trying to run tests,
		// report that to the caller so they can avoid writing a useless results dir.
		if status.ExitCode == subcommands.ExitFailure && !cfg.startedRun {
			status.FailedBeforeRun = true
		}
	}()

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to connect to %s: %v", cfg.Target, err), nil
	}

	if len(cfg.devservers) == 0 && cfg.useEphemeralDevserver {
		if err := startEphemeralDevserver(ctx, hst, cfg); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to start ephemeral devserver: %v", err), nil
		}
		defer closeEphemeralDevserver(ctx, cfg)
	}

	if cfg.downloadPrivateBundles {
		if cfg.build {
			return errorStatusf(cfg, subcommands.ExitFailure, "-downloadprivatebundles requires -build=false"), nil
		}
		if err := downloadPrivateBundles(ctx, cfg, hst); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed downloading private bundles: %v", err), nil
		}
	}

	if cfg.build {
		switch cfg.buildType {
		case localType:
			status, results = local(ctx, cfg)
		case remoteType:
			// Turn down the ephemeral devserver before running remote tests. Some remote tests
			// in the meta category run the tast command which starts yet another ephemeral devserver
			// and reverse forwarding port can conflict.
			// TODO(nya): Avoid duplicating closeEphemeralDevserver calls in this function. This will be
			// resolved on removing -buildtype flag.
			closeEphemeralDevserver(ctx, cfg)
			status, results = remote(ctx, cfg)
		default:
			// This shouldn't be reached; Config.SetFlags validates buildType.
			panic(fmt.Sprintf("Invalid build type %d", int(cfg.buildType)))
		}
	} else {
		// If we aren't rebuilding a bundle, run both local and remote tests and merge the results.
		// TODO(derat): While test runners are always supposed to report success even if tests fail,
		// it'd probably be better to run both types here even if one fails.
		if status, results = local(ctx, cfg); status.ExitCode == subcommands.ExitSuccess {
			// Turn down the ephemeral devserver before running remote tests. Some remote tests
			// in the meta category run the tast command which starts yet another ephemeral devserver
			// and reverse forwarding port can conflict.
			// TODO(nya): Avoid duplicating closeEphemeralDevserver calls in this function. This will be
			// resolved on removing -buildtype flag.
			closeEphemeralDevserver(ctx, cfg)
			var rres []TestResult
			status, rres = remote(ctx, cfg)
			results = append(results, rres...)
		}
	}

	return status, results
}
