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
	if cfg.build {
		switch cfg.buildType {
		case localType:
			status, results = local(ctx, cfg)
		case remoteType:
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
			var rres []TestResult
			status, rres = remote(ctx, cfg)
			results = append(results, rres...)
		}
	}

	// If we didn't get to the point where we started trying to run tests,
	// report that to the caller so they can avoid writing a useless results dir.
	if status.ExitCode == subcommands.ExitFailure && !cfg.startedRun {
		status.FailedBeforeRun = true
	}

	return status, results
}
