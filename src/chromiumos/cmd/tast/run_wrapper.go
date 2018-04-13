// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/cmd/tast/run"

	"github.com/google/subcommands"
)

// runWrapper is a wrapper that allows functions from the run package to be stubbed out for testing.
type runWrapper interface {
	// local calls run.Local.
	local(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult)
	// remote calls run.Remote.
	remote(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult)
	// writeResults calls run.WriteResults.
	writeResults(ctx context.Context, cfg *run.Config, results []run.TestResult) error
}

// realRunWrapper is a runWrapper implementation that calls the real functions in the run package.
type realRunWrapper struct{}

func (w realRunWrapper) local(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult) {
	return run.Local(ctx, cfg)
}

func (w realRunWrapper) remote(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult) {
	return run.Remote(ctx, cfg)
}

func (w realRunWrapper) writeResults(ctx context.Context, cfg *run.Config, results []run.TestResult) error {
	return run.WriteResults(ctx, cfg, results)
}
