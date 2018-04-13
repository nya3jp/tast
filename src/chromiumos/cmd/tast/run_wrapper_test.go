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
	lcfg, rcfg, wcfg *run.Config      // config passed to local, remote, and writeResults
	wres             []run.TestResult // results passed to writeResults

	lstat, rstat subcommands.ExitStatus // status to return from local and remote
	lres, rres   []run.TestResult       // results to return from local and remote
	werr         error                  // error to return from writeResults
}

func (w *stubRunWrapper) local(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult) {
	w.lcfg = cfg
	return w.lstat, w.lres
}

func (w *stubRunWrapper) remote(ctx context.Context, cfg *run.Config) (subcommands.ExitStatus, []run.TestResult) {
	w.rcfg = cfg
	return w.rstat, w.rres
}

func (w *stubRunWrapper) writeResults(ctx context.Context, cfg *run.Config, results []run.TestResult) error {
	w.wcfg = cfg
	w.wres = results
	return w.werr
}
