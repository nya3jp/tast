// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"chromiumos/cmd/tast/run"
	"context"

	"github.com/google/subcommands"
)

// stubRunWrapper is a stub implementation of runWrapper used for testing.
type stubRunWrapper struct {
	runCfg, writeCfg *run.Config      // config passed to run and writeResults
	writeRes         []run.TestResult // results passed to writeResults

	runStatus subcommands.ExitStatus // status to return from run
	runRes    []run.TestResult       // results to return from run
	writeErr  error                  // error to return from writeResults
}

func (w *stubRunWrapper) run(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult) {
	w.runCfg = cfg
	return w.runStatus, w.runRes
}

func (w *stubRunWrapper) writeResults(ctx context.Context, cfg *run.Config, results []run.TestResult) error {
	w.writeCfg = cfg
	w.writeRes = results
	return w.writeErr
}
