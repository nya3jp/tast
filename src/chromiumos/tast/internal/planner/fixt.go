// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"errors"
	"fmt"

	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// fixtureStatus represents a status of a fixture, as well as that of a fixture
// stack. See comments around fixtureStack for details.
type fixtureStatus int

const (
	statusRed    fixtureStatus = iota // fixture is not set up or torn down
	statusGreen                       // fixture is set up
	statusYellow                      // fixture is set up but last reset failed
)

// String converts fixtureStatus to a string for debugging.
func (s fixtureStatus) String() string {
	switch s {
	case statusRed:
		return "red"
	case statusGreen:
		return "green"
	case statusYellow:
		return "yellow"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// fixtureStack maintains a stack of fixtures and their states.
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
// fixtureStack maintains the following invariants about fixture statuses:
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
type fixtureStack struct {
	cfg *Config
	out OutputStream

	stack []*statefulFixture // fixtures on a traverse path, root to leaf
	dirty bool
}

// newFixtureStack creates a new empty fixture stack.
func newFixtureStack(cfg *Config, out OutputStream) *fixtureStack {
	return &fixtureStack{cfg: cfg, out: out}
}

// Status returns the current status of the fixture stack.
func (st *fixtureStack) Status() fixtureStatus {
	for _, f := range st.stack {
		if s := f.Status(); s != statusGreen {
			return s
		}
	}
	return statusGreen
}

// RedFixtureName returns a name of the bottom-most red fixture in the fixture
// stack.
//
// If there is no red fixture in the stack, an empty string is returned.
func (st *fixtureStack) RedFixtureName() string {
	for _, f := range st.stack {
		if f.Status() == statusRed {
			return f.Name()
		}
	}
	return ""
}

// Val returns the fixture value of the top fixture.
//
// If the fixture stack is empty or red, it returns nil.
func (st *fixtureStack) Val() interface{} {
	f := st.top()
	if f == nil || f.Status() == statusRed {
		return nil
	}
	return f.Val()
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
func (st *fixtureStack) Push(ctx context.Context, fixt *testing.Fixture) error {
	status := st.Status()
	if status == statusYellow {
		return errors.New("BUG: fixture must not be pushed to a yellow stack")
	}

	var outDir string
	if fixt.Name != "" {
		dir, err := createEntityOutDir(st.cfg.OutDir, fixt.Name)
		if err != nil {
			return err
		}
		outDir = dir
	}

	ce := &testcontext.CurrentEntity{
		OutDir:      outDir,
		ServiceDeps: fixt.ServiceDeps,
	}
	ei := fixt.EntityInfo()
	fout := newEntityOutputStream(st.out, ei)

	ctx = testing.NewContext(ctx, ce, func(msg string) { fout.Log(msg) })

	rcfg := &testing.RuntimeConfig{
		// TODO(crbug.com/1127165): Support DataDir.
		OutDir:       outDir,
		Vars:         st.cfg.Vars,
		CloudStorage: testing.NewCloudStorage(st.cfg.Devservers, st.cfg.TLWServer, st.cfg.DUTName),
		RemoteData:   st.cfg.RemoteData,
		FixtValue:    st.Val(),
		FixtCtx:      ctx,
	}

	root := testing.NewEntityRoot(ce, ei, rcfg, fout)
	f := newStatefulFixture(fixt, root, fout, st.top())
	st.stack = append(st.stack, f)

	if status == statusGreen {
		if err := st.top().RunSetUp(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Pop removes the top-most fixture from the fixture stack.
//
// If the top-most fixture is green or yellow, its TearDown method is called.
func (st *fixtureStack) Pop(ctx context.Context) error {
	f := st.top()
	st.stack = st.stack[:len(st.stack)-1]
	if f.Status() != statusRed {
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
func (st *fixtureStack) Reset(ctx context.Context) error {
	st.dirty = false

	switch st.Status() {
	case statusGreen:
	case statusRed:
		return nil
	case statusYellow:
		return errors.New("BUG: Reset called for a yellow fixture stack")
	}

	for _, f := range st.stack {
		if err := f.RunReset(ctx); err != nil {
			return err
		}
		switch f.Status() {
		case statusGreen:
		case statusRed:
			return errors.New("BUG: fixture is red after calling Reset")
		case statusYellow:
			return nil
		}
	}
	return nil
}

func (st *fixtureStack) PreTest(ctx context.Context, troot *testing.TestEntityRoot) error {
	if status := st.Status(); status != statusGreen {
		return fmt.Errorf("BUG: PreTest called for a %v fixture", status)
	}

	for _, f := range st.stack {
		if err := f.RunPreTest(ctx, troot); err != nil {
			return err
		}
	}
	return nil
}

func (st *fixtureStack) PostTest(ctx context.Context, troot *testing.TestEntityRoot) error {
	if status := st.Status(); status != statusGreen {
		return fmt.Errorf("BUG: PostTest called for a %v fixture", status)
	}

	for i := len(st.stack) - 1; i >= 0; i-- {
		f := st.stack[i]
		if err := f.RunPostTest(ctx, troot); err != nil {
			return err
		}
	}
	return nil
}

// MarkDirty marks the fixture stack dirty. It returns an error if the stack is
// already dirty.
//
// The dirty flag can be cleared by calling Reset. MarkDirty can be called
// before running a test to make sure Reset is called for sure between tests.
func (st *fixtureStack) MarkDirty() error {
	if st.dirty {
		return errors.New("BUG: MarkDirty called for a dirty stack")
	}
	st.dirty = true
	return nil
}

// top returns the stateful fixture at the top of the stack. If the stack is
// empty, nil is returned.
func (st *fixtureStack) top() *statefulFixture {
	if len(st.stack) == 0 {
		return nil
	}
	return st.stack[len(st.stack)-1]
}

// statefulFixture holds a fixture and some extra variables tracking its states.
type statefulFixture struct {
	fixt   *testing.Fixture
	root   *testing.EntityRoot
	fout   *entityOutputStream
	parent *statefulFixture

	status fixtureStatus
	val    interface{} // val returned by SetUp
}

// newStatefulFixture creates a new statefulFixture.
func newStatefulFixture(fixt *testing.Fixture, root *testing.EntityRoot, fout *entityOutputStream, parent *statefulFixture) *statefulFixture {
	return &statefulFixture{
		fixt:   fixt,
		root:   root,
		fout:   fout,
		parent: parent,
		status: statusRed,
	}
}

// Name returns the name of the fixture.
func (f *statefulFixture) Name() string {
	return f.fixt.Name
}

// Status returns the current status of the fixture.
func (f *statefulFixture) Status() fixtureStatus {
	return f.status
}

// Val returns the fixture value obtained on setup.
func (f *statefulFixture) Val() interface{} {
	return f.val
}

// RunSetUp calls SetUp of the fixture with a proper context and timeout.
func (f *statefulFixture) RunSetUp(ctx context.Context) error {
	if f.Status() != statusRed {
		return errors.New("BUG: RunSetUp called for a non-red fixture")
	}

	ctx = f.root.NewContext(ctx)
	s := f.root.NewFixtState()
	name := fmt.Sprintf("%s:SetUp", f.fixt.Name)

	f.fout.Start(s.OutDir())

	var val interface{}
	if err := safeCall(ctx, name, f.fixt.SetUpTimeout, defaultGracePeriod, errorOnPanic(s), func(ctx context.Context) {
		val = f.fixt.Impl.SetUp(ctx, s)
	}); err != nil {
		return err
	}

	if f.fout.HasError() {
		// TODO(crbug.com/1127169): Support timing log.
		f.fout.End(nil, timing.NewLog())
		return nil
	}

	f.status = statusGreen
	f.val = val
	return nil
}

// RunTearDown calls TearDown of the fixture with a proper context and timeout.
func (f *statefulFixture) RunTearDown(ctx context.Context) error {
	if f.Status() == statusRed {
		return errors.New("BUG: RunTearDown called for a red fixture")
	}

	ctx = f.root.NewContext(ctx)
	s := f.root.NewFixtState()
	name := fmt.Sprintf("%s:TearDown", f.fixt.Name)

	if err := safeCall(ctx, name, f.fixt.TearDownTimeout, defaultGracePeriod, errorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.TearDown(ctx, s)
	}); err != nil {
		return err
	}

	// TODO(crbug.com/1127169): Support timing log.
	f.fout.End(nil, timing.NewLog())

	f.status = statusRed
	f.val = nil
	return nil
}

// RunReset calls Reset of the fixture with a proper context and timeout.
func (f *statefulFixture) RunReset(ctx context.Context) error {
	if f.Status() != statusGreen {
		return errors.New("BUG: RunReset called for a non-green fixture")
	}

	ctx = f.root.NewContext(ctx)
	name := fmt.Sprintf("%s:Reset", f.fixt.Name)

	var resetErr error
	onPanic := func(val interface{}) {
		resetErr = fmt.Errorf("panic: %v", val)
	}

	if err := safeCall(ctx, name, f.fixt.ResetTimeout, defaultGracePeriod, onPanic, func(ctx context.Context) {
		resetErr = f.fixt.Impl.Reset(ctx)
	}); err != nil {
		return err
	}

	if resetErr != nil {
		f.status = statusYellow
		f.fout.Log(fmt.Sprintf("Fixture failed to reset: %v; recovering", resetErr))
	}
	return nil
}

func (f *statefulFixture) RunPreTest(ctx context.Context, troot *testing.TestEntityRoot) error {
	if status := f.Status(); status != statusGreen {
		return fmt.Errorf("BUG: RunPreTest called for a %v fixture", status)
	}

	s := troot.NewFixtTestState()
	ctx = f.newTestContext(ctx, troot, s)
	name := fmt.Sprintf("%s:PreTest", f.fixt.Name)

	return safeCall(ctx, name, f.fixt.PreTestTimeout, defaultGracePeriod, errorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.PreTest(ctx, s)
	})
}

func (f *statefulFixture) RunPostTest(ctx context.Context, troot *testing.TestEntityRoot) error {
	if status := f.Status(); status != statusGreen {
		return fmt.Errorf("BUG: RunPostTest called for a %v fixture", status)
	}

	s := troot.NewFixtTestState()
	ctx = f.newTestContext(ctx, troot, s)
	name := fmt.Sprintf("%s:PostTest", f.fixt.Name)

	return safeCall(ctx, name, f.fixt.PostTestTimeout, defaultGracePeriod, errorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.PostTest(ctx, s)
	})
}

// newTestContext returns a Context to be passed to PreTest/PostTest of a fixture.
func (f *statefulFixture) newTestContext(ctx context.Context, troot *testing.TestEntityRoot, s *testing.FixtTestState) context.Context {
	ce := &testcontext.CurrentEntity{
		// OutDir is from the test so that test hooks can save files just like tests.
		OutDir: troot.OutDir(),
		// ServiceDeps is from the fixture so that test hooks can call gRPC services
		// without relying on what tests declare in ServiceDeps.
		ServiceDeps: f.fixt.ServiceDeps,
		// SoftwareDeps is unavailable because fixtures can't declare software dependencies.
		HasSoftwareDeps: false,
	}
	return testing.NewContext(ctx, ce, func(msg string) { s.Log(msg) })
}
