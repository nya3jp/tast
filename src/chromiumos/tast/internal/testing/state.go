// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing implements public framework APIs, as well as
// framework-internal facility to run an entity.
//
// An entity is a piece of user code registered to the framework with metadata.
// Currently there are three types of entities: tests, fixtures, and services.
// Entities are registered to the framework by calling testing.Add* at the test
// bundle initialization time. Entity metadata contain various information the
// framework needs to know to call into an entity properly. For example: an
// entity name is used to mention it in other entities' metadata and command
// line arguments; dependencies (data files, services, ...) specify requirements
// that must be prepared before running an entity. When a test bundle is
// started, the framework builds an execution plan of entities according to the
// request, and prepare necessary dependencies before calling into entities.
//
// A semi-entity is a piece of user code known indirectly to the framework
// without explicit registration. Currently there are three types of
// semi-entities: preconditions, test hooks, and subtests.
// Since semi-entities are not explicitly registered to the framework, they do
// not have associated metadata. As an important consequence, semi-entities
// can't declare their own dependencies.
//
// Entities and semi-entities are implemented either as a simple user function
// or an interface (that is, a set of user functions). The framework typically
// calls into user functions with two arguments: context.Context and
// testing.*State (the exact State type varies).
//
// context.Context is associated with an entity for which a user function is
// called. Note that an associated entity might not be the one closest to
// a function being called, in terms of code location; for example,
// context.Context passed to a gRPC service method is associated with a test or
// a fixture calling into the method, not the service implementing the method.
// One can call testing.Context* functions with a given context to query
// an entity metadata (e.g. testcontext.ServiceDeps), or emit logs for an
// entity (testing.ContextLog).
//
// A new State object is created by the framework every time on calling into
// a (semi-)entity. This means that there might be multiple State objects for
// an entity at a time. To maintain states common to multiple State objects for
// the same entity, a single EntityRoot object (and additionally
// a TestEntityRoot object in the case of a test) is allocated. Root objects are
// private to the framework, and user code always access Root objects indirectly
// via State objects.
//
// Since there are many State types that provide similar but different sets of
// methods, State types typically embed mix-in types that actually implements
// API methods.
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
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/timing"
)

const (
	metaCategory  = "meta"                    // category for remote tests exercising Tast, as in "meta.TestName".
	preFailPrefix = "[Precondition failure] " // the prefix used then a precondition failure is logged.
)

// RuntimeConfig contains details about how an individual test should be run.
type RuntimeConfig struct {
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
	// FixtValue is a value returned by a parent fixture.
	// It is nil if not available.
	FixtValue interface{}
	// FixtCtx is the context that lives as long as the fixture.
	// It can be accessed only from testing.FixtState.
	FixtCtx context.Context
	// PreCtx is the context that lives as long as the precondition.
	// It can be accessed only from testing.PreState.
	PreCtx context.Context
	// Purgeable is a list of file paths which are not used for now and thus
	// can be deleted if the disk space is low.
	Purgeable []string
}

// RemoteData contains information relevant to remote entities.
type RemoteData struct {
	// Meta contains information about how the tast process was run.
	Meta *Meta
	// RPCHint contains information needed to establish gRPC connections.
	RPCHint *RPCHint
	// DUT is an SSH connection shared among remote entities.
	DUT *dut.DUT
	// CompanionDUTs are other DUTs that can be used in remote test.
	CompanionDUTs map[string]*dut.DUT
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
	// localBundleDir is the directory on the DUT where local test bundle executables are located.
	// This path is used by remote entities to invoke gRPC services in local test bundles.
	localBundleDir string
	// testVars holds all test variables and will pass to local bundle services.
	testVars map[string]string
}

// NewRPCHint create a new RPCHint struct.
func NewRPCHint(localBundleDir string, testVars map[string]string) *RPCHint {
	return &RPCHint{
		localBundleDir: localBundleDir,
		testVars:       testVars,
	}
}

// clone returns a deep copy of h.
func (h *RPCHint) clone() *RPCHint {
	hc := *h
	return &hc
}

// ExtractLocalBundleDir extracts localBundleDir from RPCHint.
func ExtractLocalBundleDir(h *RPCHint) string {
	return h.localBundleDir
}

// ExtractTestVars extracts test vars from RPCHint.
func ExtractTestVars(h *RPCHint) map[string]string {
	return h.testVars
}

// OutputStream is an interface to report streamed outputs of an entity.
// Note that planner.OutputStream is for multiple entities in contrast.
type OutputStream interface {
	// Log reports an informational log message from an entity.
	Log(msg string) error

	// Error reports an error from by an entity. An entity that reported one or
	// more errors should be considered failure.
	Error(e *jsonprotocol.Error) error
}

// NewError returns a new Error object containing reason rsn.
// skipFrames contains the number of frames to skip to get the code that's reporting
// the error: the caller should pass 0 to report its own frame, 1 to skip just its own frame,
// 2 to additionally skip the frame that called it, and so on.
func NewError(err error, fullMsg, lastMsg string, skipFrames int) *jsonprotocol.Error {
	// Also skip the NewError frame.
	skipFrames++

	// runtime.Caller starts counting stack frames at the point of the code that
	// invoked Caller.
	_, fn, ln, _ := runtime.Caller(skipFrames)

	trace := fmt.Sprintf("%s\n%s", lastMsg, stack.New(skipFrames))
	if err != nil {
		trace += fmt.Sprintf("\n%+v", err)
	}

	return &jsonprotocol.Error{
		Reason: fullMsg,
		File:   fn,
		Line:   ln,
		Stack:  trace,
	}
}

// EntityConstraints represents constraints imposed to an entity.
// For example, a test can only access runtime variables declared on its
// registration. This struct carries a list of declared runtime variables to be
// checked against in State.Var.
type EntityConstraints struct {
	vars []string
}

// EntityRoot is the root of all State objects associated with an entity.
// EntityRoot keeps track of states shared among all State objects associated
// with an entity (e.g. whether any error has been reported), as well as
// immutable entity information such as RuntimeConfig. Make sure to create State
// objects for an entity from the same EntityRoot.
// EntityRoot must be kept private to the framework.
type EntityRoot struct {
	ce  *testcontext.CurrentEntity // current entity info to be available via context.Context
	cst *EntityConstraints         // constraints for the entity
	cfg *RuntimeConfig             // details about how to run an entity
	out OutputStream               // stream to which logging messages and errors are reported

	mu       sync.Mutex // protects hasError
	hasError bool       // true if any error was reported from any associated State object
}

// NewEntityRoot returns a new EntityRoot object.
func NewEntityRoot(ce *testcontext.CurrentEntity, cst *EntityConstraints, cfg *RuntimeConfig, out OutputStream) *EntityRoot {
	return &EntityRoot{
		ce:  ce,
		cst: cst,
		cfg: cfg,
		out: out,
	}
}

func (r *EntityRoot) newGlobalMixin(errPrefix string, hasError bool) *globalMixin {
	return &globalMixin{
		entityRoot: r,
		errPrefix:  errPrefix,
		hasError:   hasError,
	}
}

func (r *EntityRoot) newVarMixin() *varMixin {
	return &varMixin{
		entityRoot: r,
	}
}

// NewFixtState creates a FixtState for a fixture.
func (r *EntityRoot) NewFixtState() *FixtState {
	return &FixtState{
		globalMixin: r.newGlobalMixin("", r.HasError()),
		varMixin:    r.newVarMixin(),
		entityRoot:  r,
	}
}

// NewContext creates a new context associated with the entity.
func (r *EntityRoot) NewContext(ctx context.Context) context.Context {
	return NewContext(ctx, r.ce, func(msg string) { r.out.Log(msg) })
}

// HasError checks if any error has been reported.
func (r *EntityRoot) HasError() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hasError
}

// recordError records that the entity has reported an error.
func (r *EntityRoot) recordError() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hasError = true
}

// TestEntityRoot is the root of all State objects associated with a test.
// TestEntityRoot is very similar to EntityRoot, but it contains additional states and
// immutable test information.
// TestEntityRoot must be kept private to the framework.
type TestEntityRoot struct {
	entityRoot *EntityRoot
	test       *TestInstance // test being run

	preValue interface{} // value returned by test.Pre.Prepare; may be nil
}

// NewTestEntityRoot returns a new TestEntityRoot object.
func NewTestEntityRoot(test *TestInstance, cfg *RuntimeConfig, out OutputStream) *TestEntityRoot {
	ce := &testcontext.CurrentEntity{
		OutDir:          cfg.OutDir,
		HasSoftwareDeps: true,
		SoftwareDeps:    test.SoftwareDeps,
		ServiceDeps:     test.ServiceDeps,
	}
	return &TestEntityRoot{
		entityRoot: NewEntityRoot(ce, test.Constraints(), cfg, out),
		test:       test,
	}
}

func (r *TestEntityRoot) newTestMixin() *testMixin {
	return &testMixin{
		testRoot: r,
	}
}

// NewTestState creates a State for a test.
func (r *TestEntityRoot) NewTestState() *State {
	return &State{
		globalMixin: r.entityRoot.newGlobalMixin("", r.HasError()),
		varMixin:    r.entityRoot.newVarMixin(),
		testMixin:   r.newTestMixin(),
		testRoot:    r,
	}
}

// NewPreState creates a PreState for a precondition.
func (r *TestEntityRoot) NewPreState() *PreState {
	return &PreState{
		globalMixin: r.entityRoot.newGlobalMixin(preFailPrefix, r.HasError()),
		varMixin:    r.entityRoot.newVarMixin(),
		testMixin:   r.newTestMixin(),
	}
}

// NewTestHookState creates a TestHookState for a test hook.
func (r *TestEntityRoot) NewTestHookState() *TestHookState {
	return &TestHookState{
		globalMixin: r.entityRoot.newGlobalMixin("", r.HasError()),
		varMixin:    r.entityRoot.newVarMixin(),
		testMixin:   r.newTestMixin(),
	}
}

// NewFixtTestState creates a FixtTestState for a test.
func (r *TestEntityRoot) NewFixtTestState() *FixtTestState {
	return &FixtTestState{
		globalMixin: r.entityRoot.newGlobalMixin("", r.HasError()),
	}
}

// NewContext creates a new context associated with the entity.
func (r *TestEntityRoot) NewContext(ctx context.Context) context.Context {
	return r.entityRoot.NewContext(ctx)
}

// HasError checks if any error has been reported.
func (r *TestEntityRoot) HasError() bool {
	return r.entityRoot.HasError()
}

// SetPreValue sets a precondition value available to the test.
func (r *TestEntityRoot) SetPreValue(val interface{}) {
	r.preValue = val
}

// OutDir returns a directory into which the entity may place arbitrary files
// that should be included with the test results.
func (r *TestEntityRoot) OutDir() string {
	return r.entityRoot.cfg.OutDir
}

// NewContext returns a context.Context to be used for the entity.
func NewContext(ctx context.Context, ec *testcontext.CurrentEntity, log func(msg string)) context.Context {
	ctx = testcontext.WithLogger(ctx, log)
	ctx = testcontext.WithCurrentEntity(ctx, ec)
	return ctx
}

// globalMixin implements common methods for all State types.
// A globalMixin object must not be shared among multiple State objects.
type globalMixin struct {
	entityRoot *EntityRoot
	errPrefix  string // prefix to be added to error messages

	mu       sync.Mutex // protects hasError
	hasError bool       // true if any error was reported from this State object or subtests' State objects
}

// CloudStorage returns a client for Google Cloud Storage.
func (s *globalMixin) CloudStorage() *CloudStorage {
	return s.entityRoot.cfg.CloudStorage
}

// RPCHint returns information needed to establish gRPC connections.
// It can only be called by remote entities.
func (s *globalMixin) RPCHint() *RPCHint {
	if s.entityRoot.cfg.RemoteData == nil {
		panic("RPCHint unavailable (running non-remote?)")
	}
	// Return a copy to make sure the entity doesn't modify the original struct.
	return s.entityRoot.cfg.RemoteData.RPCHint.clone()
}

// DUT returns a shared SSH connection.
// It can only be called by remote entities.
func (s *globalMixin) DUT() *dut.DUT {
	if s.entityRoot.cfg.RemoteData == nil {
		panic("DUT unavailable (running non-remote?)")
	}
	return s.entityRoot.cfg.RemoteData.DUT
}

// CompanionDUT returns a shared SSH connection for a companion DUT.
// It can only be called by remote entities.
func (s *globalMixin) CompanionDUT(role string) *dut.DUT {
	if s.entityRoot.cfg.RemoteData == nil {
		panic("Companion DUT unavailable (running non-remote?)")
	}
	dut, ok := s.entityRoot.cfg.RemoteData.CompanionDUTs[role]
	if !ok {
		panic(fmt.Sprintf("Companion DUT %q cannot be found", role))
	}
	return dut
}

// Log formats its arguments using default formatting and logs them.
func (s *globalMixin) Log(args ...interface{}) {
	s.entityRoot.out.Log(fmt.Sprint(args...))
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
func (s *globalMixin) Logf(format string, args ...interface{}) {
	s.entityRoot.out.Log(fmt.Sprintf(format, args...))
}

// Error formats its arguments using default formatting and marks the entity
// as having failed (using the arguments as a reason for the failure)
// while letting the entity continue execution.
func (s *globalMixin) Error(args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatError(args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.entityRoot.out.Error(e)
}

// Errorf is similar to Error but formats its arguments using fmt.Sprintf.
func (s *globalMixin) Errorf(format string, args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatErrorf(format, args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.entityRoot.out.Error(e)
}

// Fatal is similar to Error but additionally immediately ends the entity.
func (s *globalMixin) Fatal(args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatError(args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.entityRoot.out.Error(e)
	runtime.Goexit()
}

// Fatalf is similar to Fatal but formats its arguments using fmt.Sprintf.
func (s *globalMixin) Fatalf(format string, args ...interface{}) {
	s.recordError()
	fullMsg, lastMsg, err := s.formatErrorf(format, args...)
	e := NewError(err, fullMsg, lastMsg, 1)
	s.entityRoot.out.Error(e)
	runtime.Goexit()
}

// HasError reports whether the entity has already reported errors.
func (s *globalMixin) HasError() bool {
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
func (s *globalMixin) formatError(args ...interface{}) (fullMsg, lastMsg string, err error) {
	fullMsg = s.errPrefix + fmt.Sprint(args...)
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
	lastMsg = s.errPrefix + fmt.Sprint(args...)
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
func (s *globalMixin) formatErrorf(format string, args ...interface{}) (fullMsg, lastMsg string, err error) {
	fullMsg = s.errPrefix + fmt.Sprintf(format, args...)
	if len(args) >= 1 {
		if e, ok := args[len(args)-1].(error); ok {
			if m := errorfSuffix.FindStringIndex(format); m != nil {
				err = e
				args = args[:len(args)-1]
				format = format[:m[0]]
			}
		}
	}
	lastMsg = s.errPrefix + fmt.Sprintf(format, args...)
	return fullMsg, lastMsg, err
}

// recordError records that the entity has reported an error.
func (s *globalMixin) recordError() {
	s.entityRoot.recordError()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasError = true
}

// varMixin implements common methods for State types allowing to access
// runtime variables.
// A varMixin object must not be shared among multiple State objects.
type varMixin struct {
	entityRoot *EntityRoot
}

// Var returns the value for the named variable, which must have been registered via Vars.
// If a value was not supplied at runtime via the -var flag to "tast run", ok will be false.
func (s *varMixin) Var(name string) (val string, ok bool) {
	seen := false
	for _, n := range s.entityRoot.cst.vars {
		if n == name {
			seen = true
			break
		}
	}
	if !seen {
		panic(fmt.Sprintf("Variable %q was not registered in testing.Test.Vars. Try adding the line 'Vars: []string{%q},' to your testing.Test{}", name, name))
	}

	val, ok = s.entityRoot.cfg.Vars[name]
	return val, ok
}

// RequiredVar is similar to Var but aborts the entity if the named variable was not supplied.
func (s *varMixin) RequiredVar(name string) string {
	val, ok := s.Var(name)
	if !ok {
		panic(fmt.Sprintf("Required variable %q not supplied via -var or -varsfile", name))
	}
	return val
}

// testMixin implements common methods for State types associated with a test.
// A testMixin object must not be shared among multiple State objects.
type testMixin struct {
	testRoot *TestEntityRoot
}

// DataPath returns the absolute path to use to access a data file previously
// registered via Test.Data.
func (s *testMixin) DataPath(p string) string {
	for _, f := range s.testRoot.test.Data {
		if f == p {
			return filepath.Join(s.testRoot.entityRoot.cfg.DataDir, p)
		}
	}
	panic(fmt.Sprintf("Test data %q wasn't declared in definition passed to testing.AddTest", p))
}

// DataFileSystem returns an http.FileSystem implementation that serves an entity's data files.
//
//	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
//	defer srv.Close()
//	resp, err := http.Get(srv.URL+"/data_file.html")
func (s *testMixin) DataFileSystem() *dataFS { return (*dataFS)(s) }

// dataFS implements http.FileSystem.
type dataFS testMixin

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
	// Report an error for undeclared files to avoid making tests fail (or create favicon files) unnecessarily.
	// DataPath will panic if it attempts to use a file that exists but that wasn't declared as a dependency.
	path, err := func() (path string, err error) {
		defer func() {
			if recover() != nil {
				err = errors.New("not found")
			}
		}()
		return (*testMixin)(d).DataPath(name), nil
	}()
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// OutDir returns a directory into which the entity may place arbitrary files
// that should be included with the entity results.
func (s *testMixin) OutDir() string { return s.testRoot.entityRoot.cfg.OutDir }

// SoftwareDeps returns software dependencies declared in the currently running entity.
func (s *testMixin) SoftwareDeps() []string {
	return append([]string(nil), s.testRoot.test.SoftwareDeps...)
}

// ServiceDeps returns service dependencies declared in the currently running entity.
func (s *testMixin) ServiceDeps() []string {
	return append([]string(nil), s.testRoot.test.ServiceDeps...)
}

// TestName returns the name of the currently running test.
func (s *testMixin) TestName() string {
	return s.testRoot.test.Name
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
	*globalMixin
	*varMixin
	*testMixin
	testRoot *TestEntityRoot
	subtests []string // subtest names
}

// Param returns Val specified at the Param struct for the current test case.
func (s *State) Param() interface{} {
	return s.testRoot.test.Val
}

// Run starts a new subtest with an unique name. Error messages are prepended with the subtest
// name during its execution. If Fatal/Fatalf is called from inside a subtest, only that subtest
// is stopped; its parent continues. Returns true if the subtest passed.
func (s *State) Run(ctx context.Context, name string, run func(context.Context, *State)) bool {
	subtests := append([]string(nil), s.subtests...)
	subtests = append(subtests, name)
	ns := &State{
		// Set hasError to false; State for a subtest always starts with no error.
		globalMixin: s.testRoot.entityRoot.newGlobalMixin(strings.Join(subtests, "/")+": ", false),
		varMixin:    s.testRoot.entityRoot.newVarMixin(),
		testMixin:   s.testRoot.newTestMixin(),
		testRoot:    s.testRoot,
		subtests:    subtests,
	}

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
	// Bubble up errors to the parent State. Note that errors are already
	// reported to TestEntityRoot, so it is sufficient to set hasError directly.
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
func (s *State) PreValue() interface{} { return s.testRoot.preValue }

// Meta returns information about how the "tast" process used to initiate testing was run.
// It can only be called by remote tests in the "meta" category.
func (s *State) Meta() *Meta {
	if parts := strings.SplitN(s.testRoot.test.Name, ".", 2); len(parts) != 2 || parts[0] != metaCategory {
		panic(fmt.Sprintf("Meta info unavailable since test doesn't have category %q", metaCategory))
	}
	if s.testRoot.entityRoot.cfg.RemoteData == nil {
		panic("Meta info unavailable (is test non-remote?)")
	}
	// Return a copy to make sure the test doesn't modify the original struct.
	return s.testRoot.entityRoot.cfg.RemoteData.Meta.clone()
}

// FixtValue returns the fixture value if the test depends on a fixture in the same process.
// FixtValue returns nil otherwise.
func (s *State) FixtValue() interface{} {
	return s.testRoot.entityRoot.cfg.FixtValue
}

// PreState holds state relevant to the execution of a single precondition.
//
// This is a State for preconditions. See State's documentation for general
// guidance on how to treat PreState in preconditions.
type PreState struct {
	*globalMixin
	*varMixin
	*testMixin
}

// PreCtx returns a context that lives as long as the precondition.
// Can only be called from inside a precondition; it panics otherwise.
func (s *PreState) PreCtx() context.Context {
	return s.testRoot.entityRoot.cfg.PreCtx
}

// TestHookState holds state relevant to the execution of a test hook.
//
// This is a State for test hooks. See State's documentation for general
// guidance on how to treat TestHookState in test hooks.
type TestHookState struct {
	*globalMixin
	*varMixin
	*testMixin
}

// Purgeable returns a list of paths of purgeable cache files. This list may
// contain external data files downloaded previously but unused in the next
// test, and the like. Test hooks can delete those files safely without
// disrupting test execution if the disk space is low.
// Some files might be already removed, so test hooks should ignore "file not
// found" errors. Some files might have hard links, so test hooks should not
// assume that deleting an 1GB file frees 1GB space.
func (s *TestHookState) Purgeable() []string {
	return append([]string(nil), s.testRoot.entityRoot.cfg.Purgeable...)
}

// FixtState is the state the framework passes to Fixture.SetUp and Fixture.TearDown.
type FixtState struct {
	*globalMixin
	*varMixin

	entityRoot *EntityRoot
}

// FixtContext returns fixture-scoped context. i.e. the context is alive until TearDown returns.
// The context is also associated with the fixture metadata. For example,
// testing.ContextOutDir(ctx) returns the output directory allocated to the fixture.
func (s *FixtState) FixtContext() context.Context {
	return s.entityRoot.cfg.FixtCtx
}

// Param returns Val specified at the Param struct for the current fixture.
func (s *FixtState) Param() interface{} {
	// TODO(oka): Implement it.
	panic("to be implemented")
}

// ParentValue returns the parent fixture value if the fixture has a parent in the same process.
// ParentValue returns nil otherwise.
func (s *FixtState) ParentValue() interface{} {
	return s.entityRoot.cfg.FixtValue
}

// OutDir returns a directory into which the entity may place arbitrary files
// that should be included with the entity results.
func (s *FixtState) OutDir() string {
	return s.entityRoot.cfg.OutDir
}

// FixtTestState is the state the framework passes to PreTest and PostTest.
type FixtTestState struct {
	*globalMixin
}

// OutDir returns a directory into which the entity may place arbitrary files
// that should be included with the entity results.
func (s *FixtTestState) OutDir() string {
	return s.entityRoot.cfg.OutDir
}
