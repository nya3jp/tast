// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"go.chromium.org/tast/core/cmd/tast/internal/run"
	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/tastuseonly/run/resultsjson"
)

// runWrapper is a wrapper that allows functions from the run package to be stubbed out for testing.
type runWrapper interface {
	// run calls run.Run.
	run(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]*resultsjson.Result, error)
}

type globalRuntimeVarsrunWrapper interface {
	// run calls run.Run.
	GlobalRuntimeVars(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]string, error)
}

// realRunWrapper is a runWrapper implementation that calls the real functions in the run package.
type realRunWrapper struct{}

func (w realRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]*resultsjson.Result, error) {
	return run.Run(ctx, cfg, state)
}

func (w realRunWrapper) GlobalRuntimeVars(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]string, error) {
	return run.GlobalRuntimeVars(ctx, cfg, state)
}
