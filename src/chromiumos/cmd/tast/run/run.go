// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/google/subcommands"

	"chromiumos/tast/ctxutil"
)

const runErrorFilename = "run_error.txt" // text file in Config.ResDir containing error that made whole run fail

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
	ranRemote := false

	if cfg.build {
		switch cfg.buildType {
		case localType:
			status, results = local(ctx, cfg)
		case remoteType:
			status, results = remote(ctx, cfg)
			ranRemote = true
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
			ranRemote = true
		}
	}

	if status.ExitCode == subcommands.ExitFailure {
		// Provide a more-descriptive status if the SSH connection was lost.
		if !ranRemote && cfg.hst != nil && !ctxutil.DeadlineBefore(ctx, time.Now().Add(sshPingTimeout)) {
			if err := cfg.hst.Ping(ctx, sshPingTimeout); err != nil {
				msg := diagnoseSSHDrop(ctx, cfg)
				status = errorStatusf(cfg, subcommands.ExitFailure, "Lost SSH connection; %s", msg)
			}
		}

		// If we didn't get to the point where we started trying to run tests,
		// report that to the caller so they can avoid writing a useless results dir.
		if !cfg.startedRun {
			status.FailedBeforeRun = true
		}

		// Write the failure message to a file so it can be used to annotate interrupted tests.
		if err := ioutil.WriteFile(filepath.Join(cfg.ResDir, runErrorFilename),
			[]byte(status.ErrorMsg), 0644); err != nil {
			cfg.Logger.Log("Failed to write run error: ", err)
		}
	}

	return status, results
}
