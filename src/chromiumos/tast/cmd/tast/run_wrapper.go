// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/target"
)

// runWrapper is a wrapper that allows functions from the run package to be stubbed out for testing.
type runWrapper interface {
	// run calls run.Run.
	run(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (run.Status, []*resultsjson.Result)
	// writeResults calls run.WriteResults.
	writeResults(ctx context.Context, cfg *config.Config, state *config.State, results []*resultsjson.Result, complete bool, cc *target.ConnCache) error
}

// realRunWrapper is a runWrapper implementation that calls the real functions in the run package.
type realRunWrapper struct{}

func (w realRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (run.Status, []*resultsjson.Result) {
	return run.Run(ctx, cfg, state, cc)
}

func (w realRunWrapper) writeResults(ctx context.Context, cfg *config.Config, state *config.State, results []*resultsjson.Result, complete bool, cc *target.ConnCache) error {
	return runnerclient.WriteResults(ctx, cfg, state, results, complete, cc)
}
