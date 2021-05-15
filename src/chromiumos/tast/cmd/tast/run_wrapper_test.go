// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/target"
)

// stubRunWrapper is a stub implementation of runWrapper used for testing.
type stubRunWrapper struct {
	runCtx, writeCtx     context.Context       // contexts passed to run and writeResults
	runCfg, writeCfg     *config.Config        // config passed to run and writeResults
	runState, writeState *config.State         // state passed to run and writeResults
	writeRes             []*resultsjson.Result // results passed to writeResults
	writeComplete        bool                  // complete arg passed to writeResults

	runStatus run.Status            // status to return from run
	runRes    []*resultsjson.Result // results to return from run
	writeErr  error                 // error to return from writeResults
}

func (w *stubRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (run.Status, []*resultsjson.Result) {
	w.runCtx, w.runCfg, w.runState = ctx, cfg, state
	return w.runStatus, w.runRes
}

func (w *stubRunWrapper) writeResults(ctx context.Context, cfg *config.Config, state *config.State, results []*resultsjson.Result, complete bool, cc *target.ConnCache) error {
	w.writeCtx, w.writeCfg, w.writeState, w.writeRes, w.writeComplete = ctx, cfg, state, results, complete
	return w.writeErr
}
