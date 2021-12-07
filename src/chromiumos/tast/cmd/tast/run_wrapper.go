// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/run/resultsjson"
)

// runWrapper is a wrapper that allows functions from the run package to be stubbed out for testing.
type runWrapper interface {
	// run calls run.Run.
	run(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]*resultsjson.Result, error)
}

// realRunWrapper is a runWrapper implementation that calls the real functions in the run package.
type realRunWrapper struct{}

func (w realRunWrapper) run(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]*resultsjson.Result, error) {
	return run.Run(ctx, cfg, state)
}
