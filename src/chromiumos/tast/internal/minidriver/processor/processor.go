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
	"context"

	"chromiumos/tast/internal/minidriver/bundleclient"
	"chromiumos/tast/internal/run/resultsjson"
)

// DiagnoseFunc is a function called after a run error is encountered while
// reading test results to get additional information about the cause of the
// error. An empty string should be returned if additional information is
// unavailable.
// outDir is a path to a directory where extra output files can be written.
type DiagnoseFunc func(ctx context.Context, outDir string) string

// PullFunc is a function that pulls test output files to the local file system.
// It should be passed to processor.New to specify how to pull output files.
// Note that a source file path might be on a remote machine if the test runner
// is running on a remote machine. A destination file path is always on the host
// file system.
type PullFunc func(src, dst string) error

// Processor processes entity execution events.
type Processor struct {
	*preprocessor  // embed to pass through test events to preprocessor
	resultsHandler *resultsHandler
}

var (
	_ bundleclient.RunTestsOutput   = &Processor{}
	_ bundleclient.RunFixtureOutput = &Processor{}
)

// New creates a new Processor.
// resDir is a path to the directory where test execution results are written.
func New(resDir string, diagnose DiagnoseFunc, handlers []Handler) *Processor {
	resultsHandler := newResultsHandler()
	hs := append([]Handler{resultsHandler}, handlers...)
	preprocessor := newPreprocessor(resDir, diagnose, hs)
	return &Processor{
		preprocessor:   preprocessor,
		resultsHandler: resultsHandler,
	}
}

// Results returns test results.
func (p *Processor) Results() []*resultsjson.Result {
	return p.resultsHandler.Results()
}
