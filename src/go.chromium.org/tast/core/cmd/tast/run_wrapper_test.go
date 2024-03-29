// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/internal/run/resultsjson"
)

// stubRunWrapper is a stub implementation of runWrapper used for testing.
type stubRunWrapper struct {
	runCtx   context.Context         // contexts passed to run
	runCfg   *config.Config          // config passed to run
	runState *config.DeprecatedState // state passed to run

	runRes               []*resultsjson.Result // results to return from run
	runGlobalRuntimeVars []string              //results to return from GlobalRuntimeVars
	runErr               error                 // error to return from run
}

func (w *stubRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]*resultsjson.Result, error) {
	w.runCtx, w.runCfg, w.runState = ctx, cfg, state
	return w.runRes, w.runErr
}

func (w *stubRunWrapper) GlobalRuntimeVars(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]string, error) {
	w.runCtx, w.runCfg, w.runState = ctx, cfg, state
	return w.runGlobalRuntimeVars, w.runErr
}
