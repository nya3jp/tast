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

// fixtureStack maintains a stack of fixtures and their states.
//
// A fixture stack corresponds to a path from the root of a fixture tree. As we
// traverse a fixture tree, a new child fixture is pushed to the stack by Push,
// or a fixture of the lowest level is popped from the stack by Pop, calling
// their SetUp/TearDown methods as needed.
//
// A fixture is considered "alive" if its SetUp method succeeds, until its
// TearDown method is called. fixtureStack maintains the invariant that, at any
// moment, there is an integer k (0 <= k <= n; n is the number of fixtures in
// the stack) where the first k fixtures in the stack are alive and the
// remaining fixtures are non-alive.
type fixtureStack struct {
	cfg *Config
	out OutputStream

	stack []*statefulFixture // fixtures on a traverse path, root to leaf
}

// newFixtureStack creates a new empty fixture stack.
func newFixtureStack(cfg *Config) *fixtureStack {
	return &fixtureStack{cfg: cfg}
}

// Alive returns whether the top fixture is alive or not. If the stack is empty,
// Alive returns true. By the invariant of fixtureStack, the top fixture is
// alive if and only if all fixtures in the stack are alive.
func (st *fixtureStack) Alive() bool {
	if len(st.stack) == 0 {
		return true
	}
	return st.top().Alive()
}

// Val returns the fixture value of the top fixture. If the stack is empty, Val
// returns nil. Val panics if the top fixture is not alive.
func (st *fixtureStack) Val() interface{} {
	sf := st.top()
	if sf == nil {
		return nil
	}
	if !sf.Alive() {
		panic("BUG: Val called for non-alive fixture stack")
	}
	return sf.Val()
}

// Push pushes a new fixture to the fixture stack.
// f must be the top-level fixture if the stack is empty, or a child of the top
// fixture in the stack.
func (st *fixtureStack) Push(ctx context.Context, f *testing.Fixture) error {
	ce := &testing.CurrentEntity{
		// TODO: Support OutDir.
		ServiceDeps: f.ServiceDeps,
	}
	rcfg := &testing.RuntimeConfig{
		// TODO: Support DataDir.
		// TODO: Support OutDir.
		Vars:         st.cfg.Vars,
		CloudStorage: testing.NewCloudStorage(st.cfg.Devservers),
		RemoteData:   st.cfg.RemoteData,
	}
	fout := newEntityOutputStream(st.out, f.EntityInfo())
	root := testing.NewEntityRoot(ce, rcfg, fout)

	parent := st.top()
	sf := newStatefulFixture(f, root, parent)

	st.stack = append(st.stack, sf)
	if !parent.Alive() {
		return nil
	}

	if err := sf.SetUp(ctx); err != nil {
		return err
	}
	return nil
}

// Pop pops the lowest-level fixture from the fixture stack.
func (st *fixtureStack) Pop(ctx context.Context) error {
	sf := st.top()
	st.stack = st.stack[:len(st.stack)-1]
	if sf.Alive() {
		if err := sf.TearDown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Reset resets all fixtures on the path.
// If Reset fails to reset some fixtures, it recovers by calling TearDown and
// SetUp of the fixture that failed to reset, as well as those of all its
// descendant fixtures. If SetUp fails for a fixture, it and its ascendant
// fixtures are marked non-alive.
// Reset panics if any fixture in the stack are non-alive.
func (st *fixtureStack) Reset(ctx context.Context) error {
	resetLen := len(st.stack)
	for i, sf := range st.stack {
		if err := sf.Reset(ctx); err != nil {
			if err == errFixtDidNotReturn {
				return err
			}
			// TODO: Log err
			resetLen = i
			break
		}
	}

	// Tear down fixtures failed to reset.
	for i := len(st.stack) - 1; i >= resetLen; i-- {
		sf := st.stack[i]
		if err := sf.TearDown(ctx); err != nil {
			return err
		}
	}

	// Try setting up fixtures torn down above.
	for i := resetLen; i < len(st.stack); i++ {
		sf := st.stack[i]
		if err := sf.SetUp(ctx); err != nil {
			return err
		}
	}
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

// statefulFixture tracks the state of Fixture. Its methods must be called
// properly depending on the current state; otherwise they will panic.
type statefulFixture struct {
	// Immutable fields.
	fixt   *testing.Fixture
	root   *testing.EntityRoot
	parent *statefulFixture

	// Mutable fields.
	alive bool        // whether the fixture is alive
	val   interface{} // val returned by SetUp if alive
}

// newStatefulFixture creates a new non-alive statefulFixture.
func newStatefulFixture(f *testing.Fixture, root *testing.EntityRoot, parent *statefulFixture) *statefulFixture {
	return &statefulFixture{fixt: f, root: root, parent: parent}
}

// Alive returns whether the fixture is alive.
func (f *statefulFixture) Alive() bool {
	return f.alive
}

// Val returns the fixture value obtained on setup. It panics if the fixture is
// non-alive.
func (f *statefulFixture) Val() interface{} {
	if !f.alive {
		panic("BUG: Val called for non-alive fixture")
	}
	return f.val
}

// SetUp calls SetUp of the fixture. It panics if the fixture is alive.
func (f *statefulFixture) SetUp(ctx context.Context) error {
	if f.alive {
		panic("BUG: Setup called for alive fixture")
	}

	var val interface{}
	if err := runFixtStage(ctx, f.fixt.SetUpTimeout, func(ctx context.Context) {
		f.root.RunWithFixtState(ctx, func(ctx context.Context, s *testing.FixtState) {
			val = f.fixt.Impl.SetUp(ctx, s)
		})
	}); err != nil {
		return err
	}
	f.alive = true
	f.val = val
	return nil
}

// TearDown calls TearDown of the fixture. It panics if the fixture is non-alive.
func (f *statefulFixture) TearDown(ctx context.Context) error {
	if !f.alive {
		panic("BUG: TearDown called for non-alive fixture")
	}

	if err := runFixtStage(ctx, f.fixt.TearDownTimeout, func(ctx context.Context) {
		f.root.RunWithFixtState(ctx, f.fixt.Impl.TearDown)
	}); err != nil {
		return err
	}
	f.alive = false
	f.val = nil
	return nil
}

// Reset calls Reset of the fixture. It panics if the fixture is non-alive.
func (f *statefulFixture) Reset(ctx context.Context) error {
	if !f.alive {
		panic("BUG: Reset called for non-alive fixture")
	}

	var resetErr error
	if err := runFixtStage(ctx, f.fixt.ResetTimeout, func(ctx context.Context) {
		resetErr = f.fixt.Impl.Reset(ctx)
	}); err != nil {
		return err
	}
	return resetErr
}

var errFixtDidNotReturn = errors.New("fixture did not return on timeout")

// runFixtStage runs f with timeout. If f does not return after timeout plus
// exitTimeout, it returns errFixtDidNotReturn.
func runFixtStage(ctx context.Context, timeout time.Duration, f stageFunc) error {
	stages := []stage{{
		f:           f,
		ctxTimeout:  timeout,
		exitTimeout: exitTimeout,
	}}
	if !runStages(ctx, stages) {
		return errFixtDidNotReturn
	}
	return nil
}
