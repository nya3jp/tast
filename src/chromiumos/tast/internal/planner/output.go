// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"sync"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// OutputStream is an interface to report streamed outputs of multiple test runs.
// Note that testing.OutputStream is for a single test in contrast.
type OutputStream interface {
	// TestStart reports that a test t has started.
	TestStart(t *testing.TestInfo) error
	// TestLog reports an informational log message from t.
	TestLog(t *testing.TestInfo, msg string) error
	// TestError reports an error from a test t. A test that reported one or more errors should be considered failure.
	TestError(t *testing.TestInfo, e *testing.Error) error
	// TestEnd reports that a test t has ended. If skipReasons is not empty it is considered skipped.
	TestEnd(t *testing.TestInfo, skipReasons []string, timingLog *timing.Log) error
}

// testOutputStream wraps planner.OutputStream for a single test.
//
// testOutputStream implements testing.OutputStream. testOutputStream is goroutine-safe;
// it is safe to call its methods concurrently from multiple goroutines.
type testOutputStream struct {
	out OutputStream
	t   *testing.TestInfo

	mu    sync.Mutex
	ended bool
}

var _ testing.OutputStream = &testOutputStream{}

// newTestOutputStream creates testOutputStream for out and t.
func newTestOutputStream(out OutputStream, t *testing.TestInfo) *testOutputStream {
	return &testOutputStream{out: out, t: t}
}

var errAlreadyEnded = errors.New("test has already ended")

// Start reports that the test has started. It should be called exactly once.
func (w *testOutputStream) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	return w.out.TestStart(w.t)
}

// Log reports an informational log from the test.
func (w *testOutputStream) Log(msg string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	return w.out.TestLog(w.t, msg)
}

// Log reports an error from the test.
func (w *testOutputStream) Error(e *testing.Error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	return w.out.TestError(w.t, e)
}

// End reports that the test has ended. After End is called, all methods will
// fail with an error.
func (w *testOutputStream) End(skipReasons []string, timingLog *timing.Log) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	w.ended = true
	return w.out.TestEnd(w.t, skipReasons, timingLog)
}
