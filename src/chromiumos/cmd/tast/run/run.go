// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"

	"github.com/google/subcommands"
)

// Run executes or lists tests per cfg and returns the results.
func Run(ctx context.Context, cfg *Config) (status subcommands.ExitStatus, results []TestResult) {
	if !cfg.build {
		// If we aren't rebuilding a bundle, run both local and remote tests and merge the results.
		// TODO(derat): While test runners are always supposed to report success even if tests fail,
		// it'd probably be better to run both types here even if one fails.
		if status, results = local(ctx, cfg); status == subcommands.ExitSuccess {
			var rres []TestResult
			status, rres = remote(ctx, cfg)
			results = append(results, rres...)
		}
		return status, results
	}

	switch cfg.buildType {
	case localType:
		return local(ctx, cfg)
	case remoteType:
		return remote(ctx, cfg)
	default:
		return subcommands.ExitUsageError, nil
	}
}
