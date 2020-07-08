// Copyright 2020 The Chromium OS Authors. All rights reserved.
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

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/errors/stack"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/timing"
)

const (
	metaCategory  = "meta"                    // category for remote tests exercising Tast, as in "meta.TestName".
	preFailPrefix = "[Precondition failure] " // the prefix used then a precondition failure is logged.
)

// OutputStream is an interface to report streamed outputs of a test.
// Note that planner.OutputStream is for multiple tests in contrast.
type OutputStream interface {
	// Log reports an informational log message from a test.
	Log(msg string) error

	// Error reports an error from by a test. A test that reported one or more
	// errors should be considered failure.
	Error(e *Error) error
}

// Error describes an error encountered while running a test.
type Error struct {
	Reason string `json:"reason"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Stack  string `json:"stack"`
}

// RemoteData contains information relevant to remote tests.
type RemoteData struct {
	// Meta contains information about how the tast process was run.
	Meta *Meta
	// RPCHint contains information needed to establish gRPC connections.
	RPCHint *RPCHint
	// DUT is an SSH connection shared among remote tests.
	DUT *dut.DUT
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

// RPCHint contains information needed to establish gRPC connections.
type RPCHint struct {
	// LocalBundleDir is the directory on the DUT where local test bundle executables are located.
	// This path is used by remote tests to invoke gRPC services in local test bundles.
	LocalBundleDir string
}

// clone returns a deep copy of h.
func (h *RPCHint) clone() *RPCHint {
	hc := *h
	return &hc
}

// RootState is the root of all State objects.
// State objects for tests and preconditions can be obtained from this object.
//
// RootState is private to the framework; never expose it to tests.
type RootState struct {
	test *TestInstance // test being run
	out  OutputStream  // stream to which logging messages and errors are reported
	cfg  *TestConfig   // details about how to run test

	preValue interface{} // value returned by test.Pre.Prepare; may be nil

	mu     sync.Mutex
	hasErr bool // true if any error was reported
}

// baseState is a base type for State and PreState. It provides methods common
// to State and PreState.
type baseState struct {
	root     *RootState // root state for the test
	subtests []string   // subtest names; used to prefix error messages
	inPre    bool       // true if a precondition is currently executing.

	hasError bool       // true if the current subtest has encountered an error; the test fails if this is true for the initial subtest
	mu       sync.Mutex // protects HasError
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
	baseState
}

// PreState holds state relevant to the execution of a single precondition.
//
// This is a State for preconditions. See State's documentation for general
// guidance on how to treat PreState in preconditions.
type PreState struct {
	baseState
}

// TestHookState holds state relevant to the execution of a test hook.
//
// This is a State for test hooks. See State's documentation for general
// guidance on how to treat TestHookState in test hooks.
type TestHookState struct {
	baseState
}

// TestConfig contains details about how an individual test should be run.
type TestConfig struct {
	// DataDir is the directory in which the test's data files are located.
	DataDir string
	// OutDir is the directory to which the test will write output files.
	OutDir string
	// Vars contains names and values of out-of-band variables passed to tests at runtime.
	// Names must be registered in Test.Vars and values may be accessed using State.Var.
	Vars map[string]string
	// CloudStorage is a client to read files on Google Cloud Storage.
	CloudStorage *CloudStorage
	// RemoteData contains information relevant to remote tests.
	// This is nil for local tests.
	RemoteData *RemoteData
	// PreCtx is the context that lives as long as the precondition.
	// It can be accessed only from testing.PreState.
	PreCtx context.Context
}

// NewRootState returns a new RootState object.
func NewRootState(test *TestInstance, out OutputStream, cfg *TestConfig) *RootState {
	return &RootState{test: test, out: out, cfg: cfg}
}

// newTestState creates a State for a test.
func (r *RootState) newTestState() *State {
	return &State{baseState{root: r, hasError: r.HasError()}}
}

// newPreState creates a State for a precondition.
func (r *RootState) newPreState() *PreState {
	return &PreState{baseState{root: r, inPre: true, hasError: r.HasError()}}
}

// newTestHookState creates a State for a test hook.
func (r *RootState) newTestHookState() *TestHookState {
	return &TestHookState{baseState{root: r, hasError: r.HasError()}}
}

// RunWithTestState runs f, passing a Context and a State for a test.
// If f panics, it recovers and reports the error via the State.
// f is run within a goroutine to avoid making the calling goroutine exit if
// f calls s.Fatal (which calls runtime.Goexit).
func (r *RootState) RunWithTestState(ctx context.Context, f func(ctx context.Context, s *State)) {
	s := r.newTestState()
	ctx = NewContext(ctx, s.testContext(), func(msg string) { s.Log(msg) })
	runAndRecover(func() { f(ctx, s) }, s)
}

// RunWithPreState runs f, passing a Context and a State for a precondition.
// If f panics, it recovers and reports the error via the PreState.
// f is run within a goroutine to avoid making the calling goroutine exit if
// f calls s.Fatal (which calls runtime.Goexit).
func (r *RootState) RunWithPreState(ctx context.Context, f func(ctx context.Context, s *PreState)) {
	s := r.newPreState()
	ctx = NewContext(ctx, s.testContext(), func(msg string) { s.Log(msg) })
	runAndRecover(func() { f(ctx, s) }, s)
}

// RunWithTestHookState runs f, passing a Context and a State for a test hook.
// If f panics, it recovers and reports the error via the TestHookState.
// f is run within a goroutine to avoid making the calling goroutine exit if
// f calls s.Fatal (which calls runtime.Goexit).
func (r *RootState) RunWithTestHookState(ctx context.Context, f func(ctx context.Context, s *TestHookState)) {
	s := r.newTestHookState()
	ctx = NewContext(ctx, s)
	runAndRecover(func() { f(ctx, s) }, s)
}

type errorReporter interface {
	Error(args ...interface{})
}

// runAndRecover runs f synchronously, and recovers and reports an error if it panics.
// f is run within a goroutine to avoid making the calling goroutine exit if the test
// calls s.Fatal (which calls runtime.Goexit).
func runAndRecover(f func(), s errorReporter) {
	done := make(chan struct{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.Error("Panic: ", r)
			}
			done <- struct{}{}
		}()
		f()
	}()
	<-done
}

// HasError checks if any error has been reported.
func (r *RootState) HasError() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hasErr
}

// recordError records that the test has reported an error.
func (r *RootState) recordError() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hasErr = true
}

// SetPreValue sets a precondition value available to the test.
func (r *RootState) SetPreValue(val interface{}) {
	r.preValue = val
}

// NewContext returns a context.Context to be used for the test.
func NewContext(ctx context.Context, tc *TestContext, log func(msg string)) context.Context {
	ctx = logging.NewContext(ctx, log)
	ctx = WithTestContext(ctx, tc)
	return ctx
}

// testContext returns a TestContext for this state.
func (s *baseState) testContext() *TestContext {
	return &TestContext{
		OutDir:       s.OutDir(),
		SoftwareDeps: s.SoftwareDeps(),
		ServiceDeps:  s.ServiceDeps(),
	}
}

// DataPath returns the absolute path to use to access a data file previously
// registered via Test.Data.
func (s *baseState) DataPath(p string) string {
	for _, f := range s.root.test.Data {
		if f == p {
			return filepath.Join(s.root.cfg.DataDir, p)
		}
	}
	s.Fatalf("Test data %q wasn't declared in definition passed to testing.AddTest", p)
	return ""
}

// Param returns Val specified at the Param struct for the current test case.
func (s *State) Param() interface{} {
	return s.root.test.Val
}

// DataFileSystem returns an http.FileSystem implementation that serves a test's data files.
//
//	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
//	defer srv.Close()
//	resp, err := http.Get(srv.URL+"/data_file.html")
func (s *baseState) DataFileSystem() *dataFS { return (*dataFS)(s) }

// OutDir returns a directory into which the test may place arbitrary files
// that should be included with the test results.
func (s *baseState) OutDir() string { return s.root.cfg.OutDir }

// Var returns the value for the named variable, which must have been registered via Test.Vars.
// If a value was not supplied at runtime via the -var flag to "tast run", ok will be false.
func (s *baseState) Var(name string) (val string, ok bool) {
	seen := false
	for _, n := range s.root.test.Vars {
		if n == name {
			seen = true
			break
		}
	}
	if !seen {
		s.Fatalf("Variable %q was not registered in testing.Test.Vars", name)
	}

	val, ok = s.root.cfg.Vars[name]
	return val, ok
}

// RequiredVar is similar to Var but aborts the test if the named variable was not supplied.
func (s *baseState) RequiredVar(name string) string {
	val, ok := s.Var(name)
	if !ok {
		s.Fatalf("Required variable %q not supplied via -var or -varsfile", name)
	}
	return val
}

// Run starts a new subtest with an unique name. Error messages are prepended with the subtest
// name during its execution. If Fatal/Fatalf is called from inside a subtest, only that subtest
// is stopped; its parent continues. Returns true if the subtest passed.
func (s *State) Run(ctx context.Context, name string, run func(context.Context, *State)) bool {
	subtests := append([]string(nil), s.subtests...)
	subtests = append(subtests, name)
	ns := &State{baseState{root: s.root, subtests: subtests}}

	finished := make(chan struct{})

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		ctx, st := timing.Start(ctx, name)
		defer func() {
			st.End()
			close(finished)
		}()

		s.Logf("Starting subtest %s", strings.Join(subtests, "/"))
		run(ctx, ns)
	}()

	<-finished

	ns.mu.Lock()
	defer ns.mu.Unlock()
	// Bubble up failures
	if ns.hasError {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.hasError = true
	}

	return !ns.hasError
}

// PreValue returns a value supplied by the test's precondition, which must have been declared via Test.Pre
// when the test was registered. Callers should cast the returned empty interface to the correct pointer
// type; see the relevant precondition's documentation for specifics.
// nil will be returned if the test did not declare a precondition.
func (s *State) PreValue() interface{} { return s.root.preValue }

// SoftwareDeps returns software dependencies declared in the currently running test.
func (s *baseState) SoftwareDeps() []string {
	return append([]string(nil), s.root.test.SoftwareDeps...)
}

// ServiceDeps returns service dependencies declared in the currently running test.
func (s *baseState) ServiceDeps() []string {
	return append([]string(nil), s.root.test.ServiceDeps...)
}

// CloudStorage returns a client for Google Cloud Storage.
func (s *baseState) CloudStorage() *CloudStorage {
	return s.root.cfg.CloudStorage
}

// Meta returns information about how the "tast" process used to initiate testing was run.
// It can only be called by remote tests in the "meta" category.
func (s *baseState) Meta() *Meta {
	if parts := strings.SplitN(s.root.test.Name, ".", 2); len(parts) != 2 || parts[0] != metaCategory {
		s.Fatalf("Meta info unavailable since test doesn't have category %q", metaCategory)
		return nil
	}
	if s.root.cfg.RemoteData == nil {
		s.Fatal("Meta info unavailable (is test non-remote?)")
		return nil
	}
	// Return a copy to make sure the test doesn't modify the original struct.
	return s.root.cfg.RemoteData.Meta.clone()
}

// RPCHint returns information needed to establish gRPC connections.
// It can only be called by remote tests.
func (s *baseState) RPCHint() *RPCHint {
	if s.root.cfg.RemoteData == nil {
		s.Fatal("RPCHint unavailable (is test non-remote?)")
		return nil
	}
	// Return a copy to make sure the test doesn't modify the original struct.
	return s.root.cfg.RemoteData.RPCHint.clone()
}

// DUT returns a shared SSH connection.
// It can only be called by remote tests.
func (s *baseState) DUT() *dut.DUT {
	if s.root.cfg.RemoteData == nil {
		s.Fatal("DUT unavailable (is test non-remote?)")
		return nil
	}
	return s.root.cfg.RemoteData.DUT
}

// Log formats its arguments using default formatting and logs them.
func (s *baseState) Log(args ...interface{}) {
	s.root.out.Log(fmt.Sprint(args...))
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
func (s *baseState) Logf(format string, args ...interface{}) {
	s.root.out.Log(fmt.Sprintf(format, args...))
}

// Error formats its arguments using default formatting and marks the test
// as having failed (using the arguments as a reason for the failure)
// while letting the test continue execution.
func (s *baseState) Error(args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatError(args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.root.out.Error(e)
}

// Errorf is similar to Error but formats its arguments using fmt.Sprintf.
func (s *baseState) Errorf(format string, args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatErrorf(format, args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.root.out.Error(e)
}

// Fatal is similar to Error but additionally immediately ends the test.
func (s *baseState) Fatal(args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatError(args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.root.out.Error(e)
	runtime.Goexit()
}

// Fatalf is similar to Fatal but formats its arguments using fmt.Sprintf.
func (s *baseState) Fatalf(format string, args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatErrorf(format, args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.root.out.Error(e)
	runtime.Goexit()
}

// HasError reports whether the test has already reported errors.
func (s *baseState) HasError() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasError
}

// PreCtx returns a context that lives as long as the precondition.
// Can only be called from inside a precondition; it panics otherwise.
func (s *PreState) PreCtx() context.Context {
	return s.root.cfg.PreCtx
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
func (s *baseState) formatError(args ...interface{}) (fullMsg, lastMsg string, err error) {
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

	if len(s.subtests) > 0 {
		subtests := strings.Join(s.subtests, "/") + ": "

		fullMsg = subtests + fullMsg
		lastMsg = subtests + lastMsg
	}

	if s.inPre {
		fullMsg = preFailPrefix + fullMsg
		lastMsg = preFailPrefix + lastMsg
	}

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
func (s *baseState) formatErrorf(format string, args ...interface{}) (fullMsg, lastMsg string, err error) {
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

	if len(s.subtests) > 0 {
		subtests := strings.Join(s.subtests, "/") + ": "

		fullMsg = subtests + fullMsg
		lastMsg = subtests + lastMsg
	}

	if s.inPre {
		fullMsg = preFailPrefix + fullMsg
		lastMsg = preFailPrefix + lastMsg
	}

	return fullMsg, lastMsg, err
}

// recordError records that the test has reported an error.
func (s *baseState) recordError() {
	s.root.recordError()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasError = true
}

// dataFS implements http.FileSystem.
type dataFS baseState

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
	if _, err := os.Stat(filepath.Join((*baseState)(d).root.cfg.DataDir, name)); os.IsNotExist(err) {
		return nil, errors.New("not found")
	}

	return os.Open((*baseState)(d).DataPath(name))
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
