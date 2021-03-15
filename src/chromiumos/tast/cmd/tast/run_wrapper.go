// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/cmd/tast/internal/run/runnerclient"
)

// runWrapper is a wrapper that allows functions from the run package to be stubbed out for testing.
type runWrapper interface {
	// run calls run.Run.
	run(ctx context.Context, cfg *config.Config, state *config.State) (run.Status, []*jsonprotocol.EntityResult)
	// writeResults calls run.WriteResults.
	writeResults(ctx context.Context, cfg *config.Config, state *config.State, results []*jsonprotocol.EntityResult, complete bool) error
}

// realRunWrapper is a runWrapper implementation that calls the real functions in the run package.
type realRunWrapper struct{}

func (w realRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.State) (run.Status, []*jsonprotocol.EntityResult) {
	return run.Run(ctx, cfg, state)
}

func (w realRunWrapper) writeResults(ctx context.Context, cfg *config.Config, state *config.State, results []*jsonprotocol.EntityResult, complete bool) error {
	return runnerclient.WriteResults(ctx, cfg, state, results, complete)
}
