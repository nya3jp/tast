// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Key type for objects attached to context.Context objects.
type contextKeyType string

// Key used for attaching a *State to a context.Context.
var logKey contextKeyType = "log"

// Extra time granted to tests to handle timeouts.
const defaultTestCleanupTimeout = 3 * time.Second

// Error describes an error encountered while running a test.
type Error struct {
	Reason string `json:"reason"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Stack  string `json:"stack"`
}

// Output contains a piece of output (either i.e. an error or log message) from a test.
type Output struct {
	T   time.Time
	Err *Error
	Msg string
}

// State holds state relevant to the execution of a single test.
// Parts of its interface are patterned after Go's testing.T type.
// It is intended to be safe when called concurrently by multiple goroutines
// while a test is running.
type State struct {
	ch      chan Output // channel to which logging messages and errors are written
	dataDir string      // directory in which the test's data files will be located
	outDir  string      // directory to which the test should write output files

	ctx    context.Context    // context for the overall execution of the test
	cancel context.CancelFunc // cancel function associated with ctx

	tctx    context.Context    // context used by the test function
	tcancel context.CancelFunc // cancel function associated with tctx

	hasError bool       // whether the test has already reported errors or not
	mu       sync.Mutex // mutex to protect hasError
}

// NewState returns a new State object. The test's output will be streamed to ch.
// If test.CleanupTimeout is 0, a default will be used.
func NewState(ctx context.Context, test *Test, ch chan Output, dataDir, outDir string) *State {
	s := &State{
		ch:      ch,
		dataDir: dataDir,
		outDir:  outDir,
	}

	lctx := context.WithValue(ctx, logKey, s)
	if test.Timeout > 0 {
		// Test.Run uses s.ctx to watch for the test timing out. If a well-behaved test detected a timeout
		// itself using the same context and reported it as an error, we would end up with two test errors,
		// one reported by the test and one reported by Test.Run. To avoid this, add a bit more time to the
		// context used by Test.Run (s.ctx) to give the test a chance to detect the timeout (using s.tctx)
		// and exit cleanly first.
		ct := test.CleanupTimeout
		if ct == 0 {
			ct = defaultTestCleanupTimeout
		}
		s.ctx, s.cancel = context.WithTimeout(lctx, test.Timeout+ct)
		s.tctx, s.tcancel = context.WithTimeout(s.ctx, test.Timeout)
	} else {
		s.ctx, s.cancel = context.WithCancel(lctx)
		s.tctx, s.tcancel = context.WithCancel(s.ctx)
	}

	return s
}

// Context returns the context that should be used by tests.
func (s *State) Context() context.Context {
	return s.tctx
}

// DataPath returns the absolute path to use to access a data file previously
// registered via Test.Data.
func (s *State) DataPath(p string) string {
	fp := filepath.Clean(filepath.Join(s.dataDir, p))
	if !strings.HasPrefix(fp, s.dataDir+"/") {
		s.Fatalf("Invalid data path %q (expected relative path without ..)", p)
	}
	return fp
}

// OutDir returns a directory into which the test may place arbitrary files
// that should be included with the test results.
func (s *State) OutDir() string { return s.outDir }

// Log formats its arguments using default formatting and logs them.
func (s *State) Log(args ...interface{}) {
	s.ch <- Output{T: time.Now(), Msg: fmt.Sprint(args...)}
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
func (s *State) Logf(format string, args ...interface{}) {
	s.ch <- Output{T: time.Now(), Msg: fmt.Sprintf(format, args...)}
}

// Error formats its arguments using default formatting and marks the test
// as having failed (using the arguments as a reason for the failure)
// while letting the test continue execution.
func (s *State) Error(args ...interface{}) {
	s.recordError()
	e := NewError(fmt.Sprint(args...), 1)
	s.ch <- Output{T: time.Now(), Err: e}
}

// Errorf is similar to Error but formats its arguments using fmt.Sprintf.
func (s *State) Errorf(format string, args ...interface{}) {
	s.recordError()
	e := NewError(fmt.Sprintf(format, args...), 1)
	s.ch <- Output{T: time.Now(), Err: e}
}

// Fatal is similar to Error but additionally immediately ends the test.
func (s *State) Fatal(args ...interface{}) {
	s.recordError()
	e := NewError(fmt.Sprint(args...), 1)
	s.ch <- Output{T: time.Now(), Err: e}
	runtime.Goexit()
}

// Fatalf is similar to Fatal but formats its arguments using fmt.Sprintf.
func (s *State) Fatalf(format string, args ...interface{}) {
	s.recordError()
	e := NewError(fmt.Sprintf(format, args...), 1)
	s.ch <- Output{T: time.Now(), Err: e}
	runtime.Goexit()
}

// HasError reports whether the test has already reported errors.
func (s *State) HasError() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasError
}

// recordError records that the test has reported an error.
func (s *State) recordError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasError = true
}

// NewError returns a new Error object containing reason rsn.
// skipFrames contains the number of frames to skip to get the code that's reporting
// the error: the caller should pass 0 to report its own frame, 1 to skip just its own frame,
// 2 to additionally skip the frame that called it, and so on.
func NewError(rsn string, skipFrames int) *Error {
	// Also skip the NewError frame.
	skipFrames += 1

	// runtime.Caller starts counting stack frames at the point of the code that
	// invoked Caller.
	_, fn, ln, _ := runtime.Caller(skipFrames)

	// debug.Stack writes an initial line like "goroutine 22 [running]:" followed
	// by two lines per frame. It also includes itself.
	stack := string(debug.Stack())
	stackLines := strings.Split(stack, "\n")
	stack = strings.Join(stackLines[(skipFrames+1)*2+1:], "\n")

	return &Error{
		Reason: rsn,
		File:   fn,
		Line:   ln,
		Stack:  stack,
	}
}

// ContextLog formats its arguments using default formatting and logs them
// via ctx, previously provided by State.Context. It is intended to be used for
// informational logging by packages providing support for tests. Tests should
// just call State.Log or State.Logf instead.
func ContextLog(ctx context.Context, args ...interface{}) {
	if s, ok := ctx.Value(logKey).(*State); ok {
		s.Log(args...)
	}
}

// ContextLogf is similar to ContextLog but formats its arguments using fmt.Sprintf.
func ContextLogf(ctx context.Context, format string, args ...interface{}) {
	if s, ok := ctx.Value(logKey).(*State); ok {
		s.Logf(format, args...)
	}
}

// ContextOutDir is similar to OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func ContextOutDir(ctx context.Context) string {
	return ctx.Value(logKey).(*State).OutDir()
}
