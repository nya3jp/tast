// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"
	"fmt"
	"time"

	"github.com/google/subcommands"
)

// Status describes the result of a Run call.
type Status struct {
	// ExitCode contains the exit code that should be used by the tast process.
	ExitCode subcommands.ExitStatus
	// ErrorMsg describes the reason why the run failed.
	ErrorMsg string
}

// successStatus returns a Status describing a successful run.
func successStatus() Status {
	return Status{}
}

// errorStatusf returns a Status describing a failing run. format and args are combined to produce the error
// message, which is both logged to cfg.Logger and included in the returned status.
func errorStatusf(cfg *Config, code subcommands.ExitStatus, format string, args ...interface{}) Status {
	msg := fmt.Sprintf(format, args...)
	cfg.Logger.Log(msg)
	return Status{code, msg}
}

// Run executes or lists tests per cfg and returns the results.
// Messages are logged using cfg.Logger as the run progresses.
// If an error is encountered, status.ErrorMsg will be logged to cfg.Logger before returning,
// but the caller may wish to log it again later to increase its prominence if additional messages are logged.
func Run(ctx context.Context, cfg *Config) (status Status, results []TestResult) {
	if cfg.build {
		switch cfg.buildType {
		case localType:
			status, results = local(ctx, cfg)
		case remoteType:
			status, results = remote(ctx, cfg)
		default:
			// This shouldn't be reached; Config.SetFlags validates buildType.
			return Status{subcommands.ExitUsageError, ""}, nil
		}
	} else {
		// If we aren't rebuilding a bundle, run both local and remote tests and merge the results.
		// TODO(derat): While test runners are always supposed to report success even if tests fail,
		// it'd probably be better to run both types here even if one fails.
		if status, results = local(ctx, cfg); status.ExitCode == subcommands.ExitSuccess {
			var rres []TestResult
			status, rres = remote(ctx, cfg)
			results = append(results, rres...)
		}
	}

	// Provide a more-descriptive status if the SSH connection was lost.
	if status.ExitCode == subcommands.ExitFailure && cfg.hst != nil &&
		!deadlineBefore(ctx, time.Now().Add(sshPingTimeout)) {
		if err := cfg.hst.Ping(ctx, sshPingTimeout); err != nil {
			status = errorStatusf(cfg, subcommands.ExitFailure, "Lost SSH connection to %v: %v", cfg.Target, err)
		}
	}

	return status, results
}

// deadlineBefore returns true if ctx has a deadline that expires before t.
// It returns true if the deadline has already expired and false if no deadline is set.
func deadlineBefore(ctx context.Context, t time.Time) bool {
	dl, ok := ctx.Deadline()
	if !ok {
		return false
	}
	return dl.Before(t)
}
