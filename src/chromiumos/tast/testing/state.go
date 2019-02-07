// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/errors/stack"
)

// Key type for objects attached to context.Context objects.
type contextKeyType string

// Key used for attaching a *State to a context.Context.
var logKey contextKeyType = "log"

const metaCategory = "meta" // category for remote tests exercising Tast, as in "meta.TestName"

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

// Meta contains information about how the "tast" process used to initiate testing was run.
// It is used by remote tests in the "meta" category that run the tast executable to test Tast's behavior.
type Meta struct {
	// TastPath contains the absolute path to the tast executable.
	TastPath string
	// Target contains information about the DUT as "[<user>@]host[:<port>]".
	Target string
	// Flags contains flags that should be passed to the tast command's "list" and "run" subcommands.
	RunFlags []string
}

// clone returns a deep copy of m.
func (m *Meta) clone() *Meta {
	mc := *m
	mc.RunFlags = append([]string{}, m.RunFlags...)
	return &mc
}

// State holds state relevant to the execution of a single test.
//
// Parts of its interface are patterned after Go's testing.T type.
//
// State contains many pieces of data, and it's unclear which are actually being
// used when it's passed to a function. You should minimize the number of
// functions taking State as an argument. Instead you can pass State's derived
// values (e.g. s.DataPath("file.txt")) or ctx (to use with ContextLog or
// ContextOutDir etc.).
//
// It is intended to be safe when called concurrently by multiple goroutines
// while a test is running.
type State struct {
	test *Test         // test being run
	ch   chan<- Output // channel to which logging messages and errors are written
	cfg  *TestConfig   // details about how to run test

	preValue interface{} // value returned by test.Pre.Prepare; may be nil

	closed   bool       // true after close is called and ch is closed
	hasError bool       // whether the test has already reported errors or not
	mu       sync.Mutex // protects closed and hasError
}

// TestConfig contains details about how an individual test should be run.
type TestConfig struct {
	// DataDir is the directory in which the test's data files are located.
	DataDir string
	// OutDir is the directory to which the test will write output files.
	OutDir string
	// Meta contains information about how the tast process was run.
	Meta *Meta
	// PreTestFunc is run before Test.Func (and Test.Pre.Prepare, when applicable)  if non-nil.
	PreTestFunc func(context.Context, *State)
	// PostTestFunc is run after Test.Func (and Test.Pre.Cleanup, when applicable) if non-nil.
	PostTestFunc func(context.Context, *State)
	// NextTest is the test that will be run after this one.
	NextTest *Test
}

// newState returns a new State object.
func newState(test *Test, ch chan<- Output, cfg *TestConfig) *State {
	return &State{test: test, ch: ch, cfg: cfg}
}

// close is called after the test has completed to close s.ch.
func (s *State) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		close(s.ch)
		s.closed = true
	}
}

// DataPath returns the absolute path to use to access a data file previously
// registered via Test.Data.
func (s *State) DataPath(p string) string {
	for _, f := range s.test.Data {
		if f == p {
			return filepath.Join(s.cfg.DataDir, p)
		}
	}
	s.Fatalf("Test data %q wasn't declared in definition passed to testing.AddTest", p)
	return ""
}

// DataFileSystem returns an http.FileSystem implementation that serves a test's data files.
//
//	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
//	defer srv.Close()
//	resp, err := http.Get(srv.URL+"/data_file.html")
func (s *State) DataFileSystem() *dataFS { return (*dataFS)(s) }

// OutDir returns a directory into which the test may place arbitrary files
// that should be included with the test results.
func (s *State) OutDir() string { return s.cfg.OutDir }

// PreValue returns a value supplied by the test's precondition, which must have been declared via Test.Pre
// when the test was registered. Callers should cast the returned empty interface to the correct pointer
// type; see the relevant precondition's documentation for specifics.
// nil will be returned if the test did not declare a precondition.
func (s *State) PreValue() interface{} { return s.preValue }

// Meta returns information about how the "tast" process used to initiate testing was run.
// It can only be called by remote tests in the "meta" category.
func (s *State) Meta() *Meta {
	if parts := strings.SplitN(s.test.Name, ".", 2); len(parts) != 2 || parts[0] != metaCategory {
		s.Fatalf("Meta info unavailable since test doesn't have category %q", metaCategory)
		return nil
	}
	if s.cfg.Meta == nil {
		s.Fatal("Meta info unavailable (is test non-remote?)")
		return nil
	}
	// Return a copy to make sure the test doesn't modify the original struct.
	return s.cfg.Meta.clone()
}

// Log formats its arguments using default formatting and logs them.
func (s *State) Log(args ...interface{}) {
	s.writeOutput(Output{T: time.Now(), Msg: fmt.Sprint(args...)})
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
func (s *State) Logf(format string, args ...interface{}) {
	s.writeOutput(Output{T: time.Now(), Msg: fmt.Sprintf(format, args...)})
}

// Error formats its arguments using default formatting and marks the test
// as having failed (using the arguments as a reason for the failure)
// while letting the test continue execution.
func (s *State) Error(args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := formatError(args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.writeOutput(Output{T: time.Now(), Err: e})
}

// Errorf is similar to Error but formats its arguments using fmt.Sprintf.
func (s *State) Errorf(format string, args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := formatErrorf(format, args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.writeOutput(Output{T: time.Now(), Err: e})
}

// Fatal is similar to Error but additionally immediately ends the test.
func (s *State) Fatal(args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := formatError(args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.writeOutput(Output{T: time.Now(), Err: e})
	runtime.Goexit()
}

// Fatalf is similar to Fatal but formats its arguments using fmt.Sprintf.
func (s *State) Fatalf(format string, args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := formatErrorf(format, args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.writeOutput(Output{T: time.Now(), Err: e})
	runtime.Goexit()
}

// writeOutput writes o to s.ch.
// o is discarded if close has already been called since a write to a closed channel would panic.
func (s *State) writeOutput(o Output) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.ch <- o
	}
}

// HasError reports whether the test has already reported errors.
func (s *State) HasError() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasError
}

// errorSuffix matches the well-known error message suffixes for formatError.
var errorSuffix = regexp.MustCompile(`(\s*:\s*|\s+)$`)

// formatError formats an error message using fmt.Sprint.
// If the format is well-known one, such as:
//
//  formatError("Failed something: ", err)
//
// then this function extracts an error object and returns parsed error messages
// in the following way:
//
//  lastMsg = "Failed something"
//  fullMsg = "Failed something: <error message>"
func formatError(args ...interface{}) (fullMsg, lastMsg string, err error) {
	fullMsg = fmt.Sprint(args...)
	if len(args) == 1 {
		if e, ok := args[0].(error); ok {
			err = e
		}
	} else if len(args) >= 2 {
		if e, ok := args[len(args)-1].(error); ok {
			if s, ok := args[len(args)-2].(string); ok {
				if m := errorSuffix.FindStringIndex(s); m != nil {
					err = e
					args = append(args[:len(args)-2], s[:m[0]])
				}
			}
		}
	}
	lastMsg = fmt.Sprint(args...)
	return fullMsg, lastMsg, err
}

// errorfSuffix matches the well-known error message suffix for formatErrorf.
var errorfSuffix = regexp.MustCompile(`\s*:?\s*%v$`)

// formatErrorf formats an error message using fmt.Sprintf.
// If the format is the following well-known one:
//
//  formatErrorf("Failed something: %v", err)
//
// then this function extracts an error object and returns parsed error messages
// in the following way:
//
//  lastMsg = "Failed something"
//  fullMsg = "Failed something: <error message>"
func formatErrorf(format string, args ...interface{}) (fullMsg, lastMsg string, err error) {
	fullMsg = fmt.Sprintf(format, args...)
	if len(args) >= 1 {
		if e, ok := args[len(args)-1].(error); ok {
			if m := errorfSuffix.FindStringIndex(format); m != nil {
				err = e
				args = args[:len(args)-1]
				format = format[:m[0]]
			}
		}
	}
	lastMsg = fmt.Sprintf(format, args...)
	return fullMsg, lastMsg, err
}

// recordError records that the test has reported an error.
func (s *State) recordError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasError = true
}

// dataFS implements http.FileSystem.
type dataFS State

// Open opens the file at name, a path that would be passed to DataPath.
func (d *dataFS) Open(name string) (http.File, error) {
	// DataPath doesn't want a leading slash, so strip it off if present.
	if filepath.IsAbs(name) {
		var err error
		if name, err = filepath.Rel("/", name); err != nil {
			return nil, err
		}
	}

	// Chrome requests favicons automatically, but DataPath fails when asked for an unregistered file.
	// Report an error for nonexistent files to avoid making tests fail (or create favicon files) unnecessarily.
	// DataPath will still make the test fail if it attempts to use a file that exists but that wasn't
	// declared as a dependency.
	if _, err := os.Stat(filepath.Join((*State)(d).cfg.DataDir, name)); os.IsNotExist(err) {
		return nil, errors.New("not found")
	}

	return os.Open((*State)(d).DataPath(name))
}

// NewError returns a new Error object containing reason rsn.
// skipFrames contains the number of frames to skip to get the code that's reporting
// the error: the caller should pass 0 to report its own frame, 1 to skip just its own frame,
// 2 to additionally skip the frame that called it, and so on.
func NewError(err error, fullMsg, lastMsg string, skipFrames int) *Error {
	// Also skip the NewError frame.
	skipFrames++

	// runtime.Caller starts counting stack frames at the point of the code that
	// invoked Caller.
	_, fn, ln, _ := runtime.Caller(skipFrames)

	trace := fmt.Sprintf("%s\n%s", lastMsg, stack.New(skipFrames))
	if err != nil {
		trace += fmt.Sprintf("\n%+v", err)
	}

	return &Error{
		Reason: fullMsg,
		File:   fn,
		Line:   ln,
		Stack:  trace,
	}
}

// ContextLog formats its arguments using default formatting and logs them via
// ctx. It is intended to be used for informational logging by packages
// providing support for tests. If testing.State is available, just call
// State.Log or State.Logf instead.
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
func ContextOutDir(ctx context.Context) (dir string, ok bool) {
	if s, ok := ctx.Value(logKey).(*State); ok {
		return s.OutDir(), true
	}
	return "", false
}
