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
	"time"
)

// Key type for objects attached to context.Context objects.
type contextKeyType string

// Key used for attaching a *State to a context.Context.
var logKey contextKeyType = "log"

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
	ch      chan Output        // channel to which logging messages and errors are written
	dataDir string             // directory in which the test's data files will be located
	outDir  string             // directory to which the test should write output files
	ctx     context.Context    // context for the test run
	cancel  context.CancelFunc // cancel function associated with ctx
}

// NewState returns a new State object. The test's output will be streamed to ch.
func NewState(ctx context.Context, ch chan Output, dataDir, outDir string, timeout time.Duration) *State {
	s := &State{
		ch:      ch,
		dataDir: dataDir,
		outDir:  outDir,
	}

	lctx := context.WithValue(ctx, logKey, s)
	if timeout > 0 {
		s.ctx, s.cancel = context.WithTimeout(lctx, timeout)
	} else {
		s.ctx, s.cancel = context.WithCancel(lctx)
	}

	return s
}

// Context returns the context that should be used by tests.
func (s *State) Context() context.Context { return s.ctx }

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
	e := s.newError(fmt.Sprint(args...))
	s.ch <- Output{T: time.Now(), Err: e}
}

// Errorf is similar to Error but formats its arguments using fmt.Sprintf.
func (s *State) Errorf(format string, args ...interface{}) {
	e := s.newError(fmt.Sprintf(format, args...))
	s.ch <- Output{T: time.Now(), Err: e}
}

// Fatal is similar to Error but additionally immediately ends the test.
func (s *State) Fatal(args ...interface{}) {
	e := s.newError(fmt.Sprint(args...))
	s.ch <- Output{T: time.Now(), Err: e}
	runtime.Goexit()
}

// Fatalf is similar to Fatal but formats its arguments using fmt.Sprintf.
func (s *State) Fatalf(format string, args ...interface{}) {
	e := s.newError(fmt.Sprintf(format, args...))
	s.ch <- Output{T: time.Now(), Err: e}
	runtime.Goexit()
}

// newError returns a new Error object with reason rsn. It attaches additional
// information and expects that the error was initiated by the code that called
// the function that called newError.
func (s *State) newError(rsn string) *Error {
	// Skip unhelpful frames at the top of the stack,
	// namely newError and Error/Errorf/Fatal/Fatalf.
	const skipFrames = 2

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
