// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"errors"
	"time"

	"chromiumos/tast/internal/testing"
)

// fixtureStatus represents a status of a fixture, as well as that of a fixture
// stack. See comments around fixtureStack for details.
type fixtureStatus int

const (
	statusDead   fixtureStatus = iota // fixture is not set up or torn down
	statusAlive                       // fixture is set up
	statusZombie                      // fixture is set up but last reset failed
)

// fixtureStack maintains a stack of fixtures and their states.
//
// A fixture stack corresponds to a path from the root of a fixture tree. As we
// traverse a fixture tree, a new child fixture is pushed to the stack by Push,
// or a fixture of the lowest level is popped from the stack by Pop, calling
// their SetUp/TearDown methods as needed.
//
// A fixture is in exactly one of three statuses: dead, alive and zombie.
//
//  - A fixture is alive if it has been successfully set up and never failed to
//    reset so far.
//  - A fixture is zombie if it has been successfully set up but failed to
//    reset.
//  - A fixture is dead if it has been torn down.
//
// fixtureStack maintains the following invariants about fixture statuses:
//
//  1. When there is a zombie fixture in the stack, no other fixtures are dead.
//  2. When there is no zombie fixture in the stack, there is an integer k
//     (0 <= k <= n; n is the number of fixtures in the stack) where the first
//     k fixture in the stack are alive and the remaining fixtures are dead.
//
// A fixture stack can be also in exactly one of three statuses, dead, alive and
// zombie.
//
//  - A fixture stack is alive if all fixtures in the stack are alive.
//  - A fixture stack is zombie if any fixture in the stack is zombie.
//  - A fixture stack is dead if any fixture in the stack is dead.
//
// A fixture stack is alive initially. When SetUp fails on pushing a new fixture
// to the stack, the stack becomes dead until the failed fixture is popped from
// the stack. It is still possible to push more fixtures to the stack, but SetUp
// is not called for those fixtures. This behavior allows continuing to traverse
// a fixture tree despite SetUp failures to mark depending tests failed.
// When Reset fails between tests, the stack becomes zombie until the
// bottom-most zombie fixture is popped from the stack. It is not allowed to
// push more fixtures to the stack in this case. Instead, fixtures should be
// popped from the stack until the stack becomes alive.
type fixtureStack struct {
	cfg *Config
	out OutputStream

	stack []*statefulFixture // fixtures on a traverse path, root to leaf
}

// newFixtureStack creates a new empty fixture stack.
func newFixtureStack(cfg *Config, out OutputStream) *fixtureStack {
	return &fixtureStack{cfg: cfg, out: out}
}

// Status returns the current status of the fixture stack.
func (st *fixtureStack) Status() fixtureStatus {
	for _, f := range st.stack {
		if s := f.Status(); s != statusAlive {
			return s
		}
	}
	return statusAlive
}

// ZombieName returns a name of a zombie fixture in the fixture stack. If there
// is no zombie fixture in the stack, an empty string is returned.
func (st *fixtureStack) ZombieName() string {
	for _, f := range st.stack {
		if f.Status() == statusZombie {
			return f.Name()
		}
	}
	return ""
}

// Val returns the fixture value of the top fixture. If the fixture stack is
// empty or dead, it returns nil.
func (st *fixtureStack) Val() interface{} {
	f := st.top()
	if f == nil || f.Status() == statusDead {
		return nil
	}
	return f.Val()
}

// Push adds a new fixture to the top of the fixture stack.
//
// If the current fixture stack is alive, the new fixture's SetUp is called,
// and the resulting fixture stack is either alive or dead. If the current
// fixture stack is dead, the new fixture's SetUp is not called and the
// resulting fixture stack is dead. It is an error to call Push for a zombie
// fixture stack.
//
// fixt must be a base fixture if the stack is empty, otherwise a child of the
// top fixture in the stack. Since a base fixture is not expected to
//
// Push returns a fixture-scoped context, which is canceled on popping the
// fixture from the fixture stack. Context passed to this method will become a
// parent of a fixture-scoped context, and the context is used on calling
// fixture methods. This is why other methods in this type do not take Context
// as an argument.
func (st *fixtureStack) Push(ctx context.Context, fixt *testing.Fixture) (context.Context, error) {
	status := st.Status()
	if status == statusZombie {
		return nil, errors.New("BUG: fixture must not be pushed to a zombie stack")
	}

	ctx = st.pushStatefulFixture(ctx, fixt)

	if status == statusAlive {
		if err := st.top().RunSetUp(); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

func (st *fixtureStack) pushStatefulFixture(ctx context.Context, fixt *testing.Fixture) context.Context {
	var (
		ce   *testing.CurrentEntity
		rcfg *testing.RuntimeConfig
		fout testing.OutputStream
	)
	// If the stack is empty, fixt is for a base fixture. Treat a base fixture
	// as a pseudo entity having no metadata, no output directory, no logging.
	if len(st.stack) == 0 {
		ce = &testing.CurrentEntity{}
		rcfg = &testing.RuntimeConfig{FixtCtx: ctx}
		fout = newNullOutputStream()
	} else {
		ce = &testing.CurrentEntity{
			// TODO(crbug.com/1035940): Support OutDir.
			ServiceDeps: fixt.ServiceDeps,
		}
		rcfg = &testing.RuntimeConfig{
			// TODO(crbug.com/1035940): Support DataDir.
			// TODO(crbug.com/1035940): Support OutDir.
			Vars:         st.cfg.Vars,
			CloudStorage: testing.NewCloudStorage(st.cfg.Devservers),
			RemoteData:   st.cfg.RemoteData,
			FixtValue:    st.Val(),
			FixtCtx:      ctx,
		}
		fout = newEntityOutputStream(st.out, fixt.EntityInfo())
	}

	tee := newTeeOutputStream(fout)
	logger := func(msg string) { tee.Log(msg) }
	ctx, cancel := context.WithCancel(testing.NewContext(ctx, ce, logger))
	root := testing.NewEntityRoot(ce, rcfg, tee)
	f := newStatefulFixture(ctx, fixt, root, tee, cancel, st.top())
	st.stack = append(st.stack, f)
	return ctx
}

// Pop removes the top-most fixture from the fixture stack.
//
// If the top-most fixture is alive or zombie, its TearDown method is called.
//
// The fixture-scoped context associated with the fixture is canceled.
func (st *fixtureStack) Pop() error {
	f := st.top()
	st.stack = st.stack[:len(st.stack)-1]
	defer f.CancelContext()
	if f.Status() != statusDead {
		if err := f.RunTearDown(); err != nil {
			return err
		}
	}
	return nil
}

// Reset resets all fixtures on the stack if the stack is alive.
// Reset is called in bottom-to-top order. If any fixture fails to reset, the
// fixture and fixture stack becomes zombie. Unless a fixture ignores timeouts,
// this method returns no error even if a fixture becomes zombie. Thus callers
// should check Status after calling Reset to see if they can proceed to pushing
// more fixtures on the stack.
// If the stack is dead, Reset does nothing. If the stack is zombie, it is an
// error to call this method.
func (st *fixtureStack) Reset() error {
	switch st.Status() {
	case statusAlive:
	case statusDead:
		return nil
	case statusZombie:
		return errors.New("BUG: Reset called for a zombie fixture stack")
	}

	for _, f := range st.stack {
		if err := f.RunReset(); err != nil {
			return err
		}
		// If the fixture is not alive after Reset,
		switch f.Status() {
		case statusAlive:
		case statusDead:
			return errors.New("BUG: fixture is dead after calling Reset")
		case statusZombie:
			return nil
		}
	}
	return nil
}

// ResetErr returns an error encountered on calling Reset of fixtures in the
// stack.
func (st *fixtureStack) ResetErr() error {
	for _, f := range st.stack {
		switch f.Status() {
		case statusAlive:
		case statusDead:
			return errors.New("BUG: ResetErr called for a dead fixture stack")
		case statusZombie:
			return f.ResetErr()
		}
	}
	return errors.New("BUG: ResetErr called for an alive fixture stack")
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
	ctx    context.Context // fixture-scoped context
	fixt   *testing.Fixture
	root   *testing.EntityRoot
	tee    *teeOutputStream
	cancel context.CancelFunc // function to cancel a fixture-scoped context
	parent *statefulFixture

	setUp    bool        // true if the fixture is alive or zombie
	val      interface{} // val returned by SetUp if the fixture is not dead
	resetErr error       // error returned by reset
}

// newStatefulFixture creates a new non-alive statefulFixture.
func newStatefulFixture(ctx context.Context, fixt *testing.Fixture, root *testing.EntityRoot, tee *teeOutputStream, cancel context.CancelFunc, parent *statefulFixture) *statefulFixture {
	return &statefulFixture{
		ctx:    ctx,
		fixt:   fixt,
		root:   root,
		tee:    tee,
		cancel: cancel,
		parent: parent,
	}
}

// Name returns the name of the fixture.
func (f *statefulFixture) Name() string {
	return f.fixt.Name
}

// Status returns the current status of the fixture.
func (f *statefulFixture) Status() fixtureStatus {
	if !f.setUp {
		return statusDead
	}
	if f.resetErr != nil {
		return statusZombie
	}
	return statusAlive
}

// Val returns the fixture value obtained on setup.
func (f *statefulFixture) Val() interface{} {
	return f.val
}

// ResetErr returns an error returned in a last Reset call. It returns nil if
// Reset has not been called yet.
func (f *statefulFixture) ResetErr() error {
	return f.resetErr
}

// CancelContext cancels the fixture-scoped context. It must be called on
// popping this stateful fixture from a fixture stack.
func (f *statefulFixture) CancelContext() {
	f.cancel()
}

// RunSetUp calls SetUp of the fixture with a proper context and timeout.
func (f *statefulFixture) RunSetUp() error {
	if f.Status() != statusDead {
		return errors.New("BUG: RunSetUp called for a non-dead fixture")
	}

	var val interface{}
	if err := runFixtStage(f.ctx, f.fixt.SetUpTimeout, func(ctx context.Context) {
		f.root.RunWithFixtState(ctx, func(ctx context.Context, s *testing.FixtState) {
			val = f.fixt.Impl.SetUp(ctx, s)
		})
	}); err != nil {
		return err
	}

	if len(f.tee.Errors()) > 0 {
		return nil
	}

	f.setUp = true
	f.val = val
	f.resetErr = nil
	return nil
}

// RunTearDown calls TearDown of the fixture with a proper context and timeout.
func (f *statefulFixture) RunTearDown() error {
	if f.Status() == statusDead {
		return errors.New("BUG: RunTearDown called for a dead fixture")
	}

	if err := runFixtStage(f.ctx, f.fixt.TearDownTimeout, func(ctx context.Context) {
		f.root.RunWithFixtState(ctx, f.fixt.Impl.TearDown)
	}); err != nil {
		return err
	}
	f.setUp = false
	f.val = nil
	f.resetErr = nil
	return nil
}

// RunReset calls Reset of the fixture with a proper context and timeout.
func (f *statefulFixture) RunReset() error {
	if f.Status() != statusAlive {
		return errors.New("BUG: RunReset called for a non-alive fixture")
	}

	var resetErr error
	if err := runFixtStage(f.ctx, f.fixt.ResetTimeout, func(ctx context.Context) {
		resetErr = f.fixt.Impl.Reset(ctx)
	}); err != nil {
		return err
	}
	f.resetErr = resetErr
	return nil
}

// runFixtStage runs f with a timeout. It returns an error only when f does not
// return after timeout plus exitTimeout.
func runFixtStage(ctx context.Context, timeout time.Duration, f stageFunc) error {
	stages := []stage{{
		f:           f,
		ctxTimeout:  timeout,
		exitTimeout: exitTimeout,
	}}
	if !runStages(ctx, stages) {
		return errors.New("fixture did not return on timeout")
	}
	return nil
}
