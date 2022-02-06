// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner/internal/entity"
	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/internal/usercode"
)

// Config contains details about how to run fixtures.
type Config struct {
	// DataDir and other fields are subset of the fields in planner.Config.
	// See its description for details.
	DataDir           string
	OutDir            string
	Vars              map[string]string
	Service           *protocol.ServiceConfig
	BuildArtifactsURL string
	RemoteData        *testing.RemoteData
	StartFixtureName  string
	// GracePeriod specifies the grace period after fixture timeout.
	GracePeriod time.Duration
}

// InternalStack maintains a stack of fixtures in the current bundle and their states.
//
// A fixture stack corresponds to a path from the root of a fixture tree. As we
// traverse a fixture tree, a new child fixture is pushed to the stack by Push,
// or a fixture of the lowest level is popped from the stack by Pop, calling
// their SetUp/TearDown methods as needed.
//
// A fixture is in exactly one of three statuses: green, yellow, and red.
//
//  - A fixture is green if it has been successfully set up and never failed to
//    reset so far.
//  - A fixture is yellow if it has been successfully set up but failed to
//    reset.
//  - A fixture is red if it has been torn down.
//
// The following diagram illustrates the status transition of a fixture:
//
//                                   OK
//                             +-------------+
//                             v             |
//  +-----+  SetUp     OK  +-------+  Reset  |  Fail  +--------+
//  | red |---------+----->| green |---------+------->| yellow |
//  +-----+         |      +-------+                  +--------+
//   ^ ^ ^          | Fail     |                          |
//   | | +----------+          | TearDown                 | TearDown
//   | +-----------------------+                          |
//   +----------------------------------------------------+
//
// InternalStack maintains the following invariants about fixture statuses:
//
//  1. When there is a yellow fixture in the stack, no other fixtures are red.
//  2. When there is no yellow fixture in the stack, there is an integer k
//     (0 <= k <= n; n is the number of fixtures in the stack) where the first
//     k fixtures from the bottom of the stack are green and the remaining
//     fixtures are red.
//
// A fixture stack can be also in exactly one of three statuses: green, yellow,
// and red.
//
//  - A fixture stack is green if all fixtures in the stack are green.
//  - A fixture stack is yellow if any fixture in the stack is yellow.
//  - A fixture stack is red if any fixture in the stack is red.
//
// An empty fixture stack is green. When SetUp fails on pushing a new fixture
// to an green stack, the stack becomes red until the failed fixture is popped
// from the stack. It is still possible to push more fixtures to the stack, but
// SetUp is not called for those fixtures, and the stack remains red. This
// behavior allows continuing to traverse a fixture tree despite SetUp failures.
// When Reset fails between tests, the stack becomes yellow until the
// bottom-most yellow fixture is popped from the stack. It is not allowed to
// push more fixtures to the stack in this case.
//
// The following diagram illustrates the status transition of a fixture stack:
//
//                                           OK
//                                     +------------+          +-+ Push
//                                     v            |          v |
//     +--------+  Fail     Reset  +-------+  Push  |  Fail  +-----+
//  +->| yellow |<-------+---------| green |--------+------->| red |<-+
//  |  +--------+        |         +-------+                 +-----+  |
//  |       |            | OK        ^ ^ ^                      |     |
//  |       | Pop        |           | | |                      | Pop |
//  |       |            +-----------+ | |                      |     |
//  +-------+--------------------------+ +----------------------+-----+
//
// A fixture stack is clean or dirty. A stack is initially clean. A clean stack
// can be marked dirty with MarkDirty. It is an error to call MarkDirty on a
// dirty stack. The dirty flag can be cleared by Reset. MarkDirty can be called
// before running a test to make sure Reset is called for sure between tests.
type InternalStack struct {
	cfg *Config
	out output.Stream

	stack []*statefulFixture // fixtures on a traverse path, root to leaf
	dirty bool
}

// NewInternalStack creates a new empty fixture stack.
func NewInternalStack(cfg *Config, out output.Stream) *InternalStack {
	return &InternalStack{cfg: cfg, out: out}
}

// Status returns the current status of the fixture stack.
func (st *InternalStack) Status() Status {
	for _, f := range st.stack {
		if s := f.Status(); s != StatusGreen {
			return s
		}
	}
	return StatusGreen
}

// Errors returns errors to be reported for tests depending on this fixture
// stack.
//
// If there is no red fixture in the stack, an empty slice is returned.
// Otherwise, this function returns a slice of error messages to be reported
// for tests depending on the fixture stack. An error message is formatted in
// the following way:
//
//  [Fixture failure] (fixture name): (original error message)
func (st *InternalStack) Errors() []*protocol.Error {
	for _, f := range st.stack {
		if f.Status() == StatusRed {
			return f.Errors()
		}
	}
	return nil
}

// Val returns the fixture value of the top fixture.
//
// If the fixture stack is empty or red, it returns nil.
func (st *InternalStack) Val() interface{} {
	if len(st.stack) == 0 {
		return nil
	}
	if st.Status() == StatusRed {
		return nil
	}
	return st.top().Val()
}

// Push adds a new fixture to the top of the fixture stack.
//
// If the current fixture stack is green, the new fixture's SetUp is called,
// and the resulting fixture stack is either green or red.
//
// If the current fixture stack is red, the new fixture's SetUp is not called
// and the resulting fixture stack is red.
//
// It is an error to call Push for a yellow fixture stack.
func (st *InternalStack) Push(ctx context.Context, fixt *testing.FixtureInstance) error {
	status := st.Status()
	if status == StatusYellow {
		return errors.New("BUG: fixture must not be pushed to a yellow stack")
	}

	var outDir string
	if fixt.Name != "" {
		dir, err := entity.CreateOutDir(st.cfg.OutDir, fixt.Name)
		if err != nil {
			return err
		}
		outDir = dir
	}

	ce := &testcontext.CurrentEntity{
		OutDir:      outDir,
		ServiceDeps: fixt.ServiceDeps,
		Labels:      fixt.Labels,
	}
	ei := fixt.EntityProto()
	fout := output.NewEntityStream(st.out, ei)

	ctx = testing.NewContext(ctx, ce, logging.NewFuncSink(func(msg string) { fout.Log(msg) }))
	root := testing.NewEntityRoot(
		ce,
		fixt.Constraints(),
		st.newRuntimeConfig(ctx, outDir, fixt),
		fout,
		testing.NewEntityCondition(),
	)
	f := newStatefulFixture(fixt, root, fout, st.cfg)
	st.stack = append(st.stack, f)

	if status == StatusGreen {
		if err := st.top().RunSetUp(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Pop removes the top-most fixture from the fixture stack.
//
// If the top-most fixture is green or yellow, its TearDown method is called.
func (st *InternalStack) Pop(ctx context.Context) error {
	f := st.top()
	st.stack = st.stack[:len(st.stack)-1]
	if f.Status() != StatusRed {
		if err := f.RunTearDown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Reset resets all fixtures on the stack if the stack is green.
//
// Reset clears the dirty flag of the stack.
//
// Reset is called in bottom-to-top order. If any fixture fails to reset, the
// fixture and fixture stack becomes yellow.
//
// Unless the fixture execution is abandoned, this method returns success even
// if Reset returns an error and the fixture becomes yellow. Callers should
// check Status after calling Reset to see if they can proceed to pushing more
// fixtures on the stack.
//
// If the stack is red, Reset does nothing. If the stack is yellow, it is an
// error to call this method.
func (st *InternalStack) Reset(ctx context.Context) error {
	st.dirty = false

	switch st.Status() {
	case StatusGreen:
	case StatusRed:
		return nil
	case StatusYellow:
		return errors.New("BUG: Reset called for a yellow fixture stack")
	}

	for _, f := range st.stack {
		if err := f.RunReset(ctx); err != nil {
			return err
		}
		switch f.Status() {
		case StatusGreen:
		case StatusRed:
			return errors.New("BUG: fixture is red after calling Reset")
		case StatusYellow:
			return nil
		}
	}
	return nil
}

// PreTest runs PreTests on the fixtures.
// It returns a post test hook that runs PostTests on the fixtures.
func (st *InternalStack) PreTest(ctx context.Context, outDir string, out testing.OutputStream, condition *testing.EntityCondition) (func(ctx context.Context) error, error) {
	if status := st.Status(); status != StatusGreen {
		return nil, fmt.Errorf("BUG: PreTest called for a %v fixture", status)
	}

	var postTests []func(context.Context) error
	for _, f := range st.stack {
		rcfg := st.newRuntimeConfig(ctx, outDir, f.fixt)
		pt, err := f.RunPreTest(ctx, rcfg, out, condition)
		if err != nil {
			return nil, err
		}
		postTests = append(postTests, pt)
	}

	return func(ctx context.Context) error {
		if status := st.Status(); status != StatusGreen {
			return fmt.Errorf("BUG: PostTest called for a %v fixture", status)
		}
		for i := len(postTests) - 1; i >= 0; i-- {
			if err := postTests[i](ctx); err != nil {
				return err
			}
		}
		return nil
	}, nil
}

// MarkDirty marks the fixture stack dirty. It returns an error if the stack is
// already dirty.
//
// The dirty flag can be cleared by calling Reset. MarkDirty can be called
// before running a test to make sure Reset is called for sure between tests.
func (st *InternalStack) MarkDirty() error {
	if st.dirty {
		return errors.New("BUG: MarkDirty called for a dirty stack")
	}
	st.dirty = true
	return nil
}

// top returns the stateful fixture at the top of the stack.
func (st *InternalStack) top() *statefulFixture {
	if len(st.stack) == 0 {
		panic("BUG: top called for an empty stack")
	}
	return st.stack[len(st.stack)-1]
}

func (st *InternalStack) newRuntimeConfig(ctx context.Context, outDir string, fixt *testing.FixtureInstance) *testing.RuntimeConfig {
	return &testing.RuntimeConfig{
		DataDir: filepath.Join(st.cfg.DataDir, testing.RelativeDataDir(fixt.Pkg)),
		OutDir:  outDir,
		Vars:    st.cfg.Vars,
		CloudStorage: testing.NewCloudStorage(
			st.cfg.Service.GetDevservers(),
			st.cfg.Service.GetTlwServer(),
			st.cfg.Service.GetTlwSelfName(),
			st.cfg.Service.GetDutServer(),
			st.cfg.BuildArtifactsURL,
		),
		RemoteData: st.cfg.RemoteData,
		FixtValue:  st.Val(),
		FixtCtx:    ctx,
	}
}

// statefulFixture holds a fixture and some extra variables tracking its states.
type statefulFixture struct {
	cfg *Config

	fixt *testing.FixtureInstance
	root *testing.EntityRoot
	fout *output.EntityStream

	status Status
	errs   []*protocol.Error
	val    interface{} // val returned by SetUp
}

// newStatefulFixture creates a new statefulFixture.
func newStatefulFixture(fixt *testing.FixtureInstance, root *testing.EntityRoot, fout *output.EntityStream, cfg *Config) *statefulFixture {
	return &statefulFixture{
		cfg:    cfg,
		fixt:   fixt,
		root:   root,
		fout:   fout,
		status: StatusRed,
	}
}

// Name returns the name of the fixture.
func (f *statefulFixture) Name() string {
	return f.fixt.Name
}

// Status returns the current status of the fixture.
func (f *statefulFixture) Status() Status {
	return f.status
}

// Errors returns errors to be reported for tests depending on the fixture.
//
// If SetUp has not been called for the fixture, an empty slice is returned.
// Otherwise, this function returns a slice of error messages to be reported
// for tests depending on the fixture. An error message is formatted in the
// following way:
//
//  [Fixture failure] (fixture name): (original error message)
func (f *statefulFixture) Errors() []*protocol.Error {
	return f.errs
}

// Val returns the fixture value obtained on setup.
func (f *statefulFixture) Val() interface{} {
	return f.val
}

// RunSetUp calls SetUp of the fixture with a proper context and timeout.
func (f *statefulFixture) RunSetUp(ctx context.Context) error {
	if f.Status() != StatusRed {
		return errors.New("BUG: RunSetUp called for a non-red fixture")
	}

	ctx = f.root.NewContext(ctx)
	s := f.root.NewFixtState()
	name := fmt.Sprintf("%s:SetUp", f.fixt.Name)

	f.fout.Start(s.OutDir())

	var val interface{}
	if err := usercode.SafeCall(ctx, name, f.fixt.SetUpTimeout, f.cfg.GracePeriod, usercode.ErrorOnPanic(s), func(ctx context.Context) {
		entity.PreCheck(f.fixt.Data, s)
		if s.HasError() {
			return
		}

		val = f.fixt.Impl.SetUp(ctx, s)
	}); err != nil {
		return err
	}
	fixtName := f.fixt.Name
	if fixtName == "" {
		fixtName = f.cfg.StartFixtureName
	}
	f.errs = rewriteErrorsForTest(f.fout.Errors(), fixtName)
	if len(f.errs) > 0 {
		// TODO(crbug.com/1127169): Support timing log.
		f.fout.End(nil, timing.NewLog())
		return nil
	}

	f.status = StatusGreen
	f.val = val
	return nil
}

// RunTearDown calls TearDown of the fixture with a proper context and timeout.
func (f *statefulFixture) RunTearDown(ctx context.Context) error {
	if f.Status() == StatusRed {
		return errors.New("BUG: RunTearDown called for a red fixture")
	}

	ctx = f.root.NewContext(ctx)
	s := f.root.NewFixtState()
	name := fmt.Sprintf("%s:TearDown", f.fixt.Name)

	if err := usercode.SafeCall(ctx, name, f.fixt.TearDownTimeout, f.cfg.GracePeriod, usercode.ErrorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.TearDown(ctx, s)
	}); err != nil {
		return err
	}

	// TODO(crbug.com/1127169): Support timing log.
	f.fout.End(nil, timing.NewLog())

	f.status = StatusRed
	f.val = nil
	return nil
}

// RunReset calls Reset of the fixture with a proper context and timeout.
func (f *statefulFixture) RunReset(ctx context.Context) error {
	if f.Status() != StatusGreen {
		return errors.New("BUG: RunReset called for a non-green fixture")
	}

	ctx = f.root.NewContext(ctx)
	name := fmt.Sprintf("%s:Reset", f.fixt.Name)

	var resetErr error
	onPanic := func(val interface{}) {
		resetErr = fmt.Errorf("panic: %v", val)
	}

	if err := usercode.SafeCall(ctx, name, f.fixt.ResetTimeout, f.cfg.GracePeriod, onPanic, func(ctx context.Context) {
		resetErr = f.fixt.Impl.Reset(ctx)
	}); err != nil {
		return err
	}

	if resetErr != nil {
		f.status = StatusYellow
		f.fout.Log(fmt.Sprintf("Fixture failed to reset: %v; recovering", resetErr))
	}
	return nil
}

// RunPreTest runs PreTest on the fixture. It returns a post test hook.
func (f *statefulFixture) RunPreTest(ctx context.Context, rcfg *testing.RuntimeConfig, out testing.OutputStream, condition *testing.EntityCondition) (func(ctx context.Context) error, error) {
	if status := f.Status(); status != StatusGreen {
		return nil, fmt.Errorf("BUG: RunPreTest called for a %v fixture", status)
	}

	doNothing := func(context.Context) error { return nil }
	if condition.HasError() {
		// If errors are already reported, PreTest and PostTest will not run.
		return doNothing, nil
	}

	froot := testing.NewFixtTestEntityRoot(f.fixt, rcfg, out, condition)
	ctx = f.newTestContext(ctx, froot, froot.LogSink())
	s := froot.NewFixtTestState(ctx)
	name := fmt.Sprintf("%s:PreTest", f.fixt.Name)
	if err := usercode.SafeCall(ctx, name, f.fixt.PreTestTimeout, f.cfg.GracePeriod, usercode.ErrorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.PreTest(ctx, s)
	}); err != nil {
		return nil, err
	}
	if condition.HasError() {
		// If errors are reported in PreTest, PostTest will not run.
		return doNothing, nil
	}
	return func(ctx context.Context) error {
		return f.runPostTest(ctx, rcfg, out, condition)
	}, nil
}

func (f *statefulFixture) runPostTest(ctx context.Context, rcfg *testing.RuntimeConfig, out testing.OutputStream, condition *testing.EntityCondition) error {
	if status := f.Status(); status != StatusGreen {
		return fmt.Errorf("BUG: RunPostTest called for a %v fixture", status)
	}

	froot := testing.NewFixtTestEntityRoot(f.fixt, rcfg, out, condition)
	ctx = f.newTestContext(ctx, froot, froot.LogSink())
	s := froot.NewFixtTestState(ctx)
	name := fmt.Sprintf("%s:PostTest", f.fixt.Name)

	if err := usercode.SafeCall(ctx, name, f.fixt.PostTestTimeout, f.cfg.GracePeriod, usercode.ErrorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.PostTest(ctx, s)
	}); err != nil {
		return err
	}
	return nil
}

// newTestContext returns a Context to be passed to PreTest/PostTest of a fixture.
func (f *statefulFixture) newTestContext(ctx context.Context, troot *testing.FixtTestEntityRoot, sink logging.Sink) context.Context {
	ce := &testcontext.CurrentEntity{
		// OutDir is from the test so that test hooks can save files just like tests.
		OutDir: troot.OutDir(),
		// ServiceDeps is from the fixture so that test hooks can call gRPC services
		// without relying on what tests declare in ServiceDeps.
		ServiceDeps: f.fixt.ServiceDeps,
		// SoftwareDeps is unavailable because fixtures can't declare software dependencies.
		HasSoftwareDeps: false,
		// Labels is from the fixture so that each user function can check them.
		Labels: f.fixt.Labels,
	}
	return testing.NewContext(ctx, ce, sink)
}

// rewriteErrorsForTest rewrites error messages reported by a fixture to be
// suitable for reporting for tests depending on the fixture.
func rewriteErrorsForTest(errs []*protocol.Error, fixtureName string) []*protocol.Error {
	newErrs := make([]*protocol.Error, len(errs))
	for i, e := range errs {
		reason := e.GetReason()
		if !strings.HasPrefix(reason, "[Fixture failure]") {
			reason = fmt.Sprintf("[Fixture failure] %s: %s", fixtureName, reason)
		}
		newErrs[i] = &protocol.Error{
			Reason:   reason,
			Location: e.GetLocation(),
		}
	}
	return newErrs
}
