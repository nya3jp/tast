// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"errors"
	"fmt"

	"chromiumos/tast/internal/testing"
)

// fixtureStatus represents a status of a fixture, as well as that of a fixture
// stack. See comments around fixtureStack for details.
type fixtureStatus int

const (
	statusRed    fixtureStatus = iota // fixture is not set up or torn down
	statusGreen                       // fixture is set up
	statusOrange                      // fixture is set up but last reset failed
)

// fixtureStack maintains a stack of fixtures and their states.
//
// A fixture stack corresponds to a path from the root of a fixture tree. As we
// traverse a fixture tree, a new child fixture is pushed to the stack by Push,
// or a fixture of the lowest level is popped from the stack by Pop, calling
// their SetUp/TearDown methods as needed.
//
// A fixture is in exactly one of three statuses: green, orange, and red.
//
//  - A fixture is green if it has been successfully set up and never failed to
//    reset so far.
//  - A fixture is orange if it has been successfully set up but failed to
//    reset.
//  - A fixture is red if it has been torn down.
//
// fixtureStack maintains the following invariants about fixture statuses:
//
//  1. When there is an orange fixture in the stack, no other fixtures are red.
//  2. When there is no orange fixture in the stack, there is an integer k
//     (0 <= k <= n; n is the number of fixtures in the stack) where the first
//     k fixture in the stack are green and the remaining fixtures are red.
//
// A fixture stack can be also in exactly one of three statuses: green, orange,
// and red.
//
//  - A fixture stack is green if all fixtures in the stack are green.
//  - A fixture stack is orange if any fixture in the stack is orange.
//  - A fixture stack is red if any fixture in the stack is red.
//
// An empty fixture stack is green. When SetUp fails on pushing a new fixture
// to an green stack, the stack becomes red until the failed fixture is popped
// from the stack. It is still possible to push more fixtures to the stack, but
// SetUp is not called for those fixtures, and the stack remains red. This
// behavior allows continuing to traverse a fixture tree despite SetUp failures.
// When Reset fails between tests, the stack becomes orange until the
// bottom-most orange fixture is popped from the stack. It is not allowed to
// push more fixtures to the stack in this case.
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
		if s := f.Status(); s != statusGreen {
			return s
		}
	}
	return statusGreen
}

// OrangeFixtureName returns a name of an orange fixture in the fixture stack.
//
// If there is no orange fixture in the stack, an empty string is returned.
func (st *fixtureStack) OrangeFixtureName() string {
	for _, f := range st.stack {
		if f.Status() == statusOrange {
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
// It is an error to call Push for an orange fixture stack.
func (st *fixtureStack) Push(ctx context.Context, fixt *testing.Fixture) error{
	status := st.Status()
	if status == statusOrange {
		return errors.New("BUG: fixture must not be pushed to an orange stack")
	}

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
	root := testing.NewEntityRoot(ce, rcfg, tee)
	f := newStatefulFixture(fixt, root, tee, st.top())
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
// If the top-most fixture is green or orange, its TearDown method is called.
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
// Reset is called in bottom-to-top order. If any fixture fails to reset, the
// fixture and fixture stack becomes orange.
//
// Unless the fixture execution is abandoned, this method returns success even
// if Reset returns an error and the fixture becomes orange. Callers should
// check Status after calling Reset to see if they can proceed to pushing more
// fixtures on the stack.
//
// If the stack is red, Reset does nothing. If the stack is orange, it is an
// error to call this method.
func (st *fixtureStack) Reset(ctx context.Context) error {
	switch st.Status() {
	case statusGreen:
	case statusRed:
		return nil
	case statusOrange:
		return errors.New("BUG: Reset called for an orange fixture stack")
	}

	for _, f := range st.stack {
		if err := f.RunReset(ctx); err != nil {
			return err
		}
		// If the fixture is not green after Reset,
		switch f.Status() {
		case statusGreen:
		case statusRed:
			return errors.New("BUG: fixture is red after calling Reset")
		case statusOrange:
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
		case statusGreen:
		case statusRed:
			return errors.New("BUG: ResetErr called for a red fixture stack")
		case statusOrange:
			return f.ResetErr()
		}
	}
	return errors.New("BUG: ResetErr called for an green fixture stack")
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
	tee    *teeOutputStream
	parent *statefulFixture

	setUp    bool        // true if the fixture is green or orange
	val      interface{} // val returned by SetUp if the fixture is not red
	resetErr error       // error returned by reset
}

// newStatefulFixture creates a new statefulFixture.
func newStatefulFixture(fixt *testing.Fixture, root *testing.EntityRoot, tee *teeOutputStream, parent *statefulFixture) *statefulFixture {
	return &statefulFixture{
		fixt:   fixt,
		root:   root,
		tee:    tee,
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
		return statusRed
	}
	if f.resetErr != nil {
		return statusOrange
	}
	return statusGreen
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

// RunSetUp calls SetUp of the fixture with a proper context and timeout.
func (f *statefulFixture) RunSetUp(ctx context.Context) error {
	if f.Status() != statusRed {
		return errors.New("BUG: RunSetUp called for a non-red fixture")
	}

	ctx = f.root.NewContext(ctx)
	s := f.root.NewFixtState()
	name := fmt.Sprintf("%s:SetUp", f.fixt.Name)

	var val interface{}
	if err := safeCall(ctx, name, f.fixt.SetUpTimeout, defaultExitTimeout, errorOnPanic(s), func(ctx context.Context) {
		val = f.fixt.Impl.SetUp(ctx, s)
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
func (f *statefulFixture) RunTearDown(ctx context.Context) error {
	if f.Status() == statusRed {
		return errors.New("BUG: RunTearDown called for a red fixture")
	}

	ctx = f.root.NewContext(ctx)
	s := f.root.NewFixtState()
	name := fmt.Sprintf("%s:TearDown", f.fixt.Name)

	if err := safeCall(ctx, name, f.fixt.TearDownTimeout, defaultExitTimeout, errorOnPanic(s), func(ctx context.Context) {
		f.fixt.Impl.TearDown(ctx, s)
	}); err != nil {
		return err
	}

	f.setUp = false
	f.val = nil
	f.resetErr = nil
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

	if err := safeCall(ctx, name, f.fixt.ResetTimeout, defaultExitTimeout, onPanic, func(ctx context.Context) {
		resetErr = f.fixt.Impl.Reset(ctx)
	}); err != nil {
		return err
	}
	f.resetErr = resetErr
	return nil
}
