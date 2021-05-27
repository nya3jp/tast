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
	runCtx   context.Context // contexts passed to run
	runCfg   *config.Config  // config passed to run
	runState *config.State   // state passed to run

	runStatus run.Status            // status to return from run
	runRes    []*resultsjson.Result // results to return from run
}

func (w *stubRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (run.Status, []*resultsjson.Result) {
	w.runCtx, w.runCfg, w.runState = ctx, cfg, state
	return w.runStatus, w.runRes
}
