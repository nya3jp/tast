// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/jsonprotocol"
)

// stubRunWrapper is a stub implementation of runWrapper used for testing.
type stubRunWrapper struct {
	runCtx, writeCtx     context.Context              // contexts passed to run and writeResults
	runCfg, writeCfg     *config.Config               // config passed to run and writeResults
	runState, writeState *config.State                // state passed to run and writeResults
	writeRes             []*jsonprotocol.EntityResult // results passed to writeResults
	writeComplete        bool                         // complete arg passed to writeResults

	runStatus run.Status                   // status to return from run
	runRes    []*jsonprotocol.EntityResult // results to return from run
	writeErr  error                        // error to return from writeResults
}

func (w *stubRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.State) (run.Status, []*jsonprotocol.EntityResult) {
	w.runCtx, w.runCfg, w.runState = ctx, cfg, state
	return w.runStatus, w.runRes
}

func (w *stubRunWrapper) writeResults(ctx context.Context, cfg *config.Config, state *config.State, results []*jsonprotocol.EntityResult, complete bool) error {
	w.writeCtx, w.writeCfg, w.writeState, w.writeRes, w.writeComplete = ctx, cfg, state, results, complete
	return w.writeErr
}
