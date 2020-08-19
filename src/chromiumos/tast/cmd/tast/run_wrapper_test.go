// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run"
)

// stubRunWrapper is a stub implementation of runWrapper used for testing.
type stubRunWrapper struct {
	runCtx, writeCtx context.Context     // contexts passed to run and writeResults
	runCfg, writeCfg *run.Config         // config passed to run and writeResults
	writeRes         []*run.EntityResult // results passed to writeResults
	writeComplete    bool                // complete arg passed to writeResults

	runStatus run.Status          // status to return from run
	runRes    []*run.EntityResult // results to return from run
	writeErr  error               // error to return from writeResults
}

func (w *stubRunWrapper) run(ctx context.Context, cfg *run.Config) (run.Status, []*run.EntityResult) {
	w.runCtx, w.runCfg = ctx, cfg
	return w.runStatus, w.runRes
}

func (w *stubRunWrapper) writeResults(ctx context.Context, cfg *run.Config, results []*run.EntityResult, complete bool) error {
	w.writeCtx, w.writeCfg, w.writeRes, w.writeComplete = ctx, cfg, results, complete
	return w.writeErr
}
