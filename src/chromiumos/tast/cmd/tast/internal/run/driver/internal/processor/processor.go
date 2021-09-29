// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package processor provides the test execution event processor.
//
// Processor consists of single Preprocessor and multiple Handlers. Processor
// first passes through test execution events to Preprocessor, which in turn
// pass them down to Handlers.
//
// Preprocessor performs several preprocessing. One of the most important ones
// is to ensure consistency of test events by possibly generating artificial
// test events. For example, in case of runner crashes, we may not receive an
// EntityEnd event corresponding to an EntityStart event received earlier. In
// such case, Preprocessor generates artificial EntityError/EntityEnd events so
// that every Handler doesn't need to handle such exceptional cases.
//
// Handlers implement general processing. Handlers are isolated from each other,
// that is, a behavior of one Handler does not affect that of another Handler.
// Most processing should go to Handlers instead of Preprocessor unless it is
// strictly necessary.
package processor

import (
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/bundleclient"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
)

// Processor processes entity execution events.
type Processor struct {
	*preprocessor  // embed to pass through test events to preprocessor
	resultsHandler *resultsHandler
}

var (
	_ runnerclient.RunTestsOutput   = &Processor{}
	_ bundleclient.RunFixtureOutput = &Processor{}
)

// New creates a new Processor.
// resDir is a path to the directory where test execution results are written.
func New(resDir string) *Processor {
	resultsHandler := newResultsHandler()
	preprocessor := newPreprocessor(resDir, []handler{
		resultsHandler,
	})
	return &Processor{
		preprocessor:   preprocessor,
		resultsHandler: resultsHandler,
	}
}

// Results returns test results.
func (p *Processor) Results() []*resultsjson.Result {
	return p.resultsHandler.Results()
}
