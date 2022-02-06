// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// CombinedStack combines two stacks and show them as a single stack.
type CombinedStack struct {
	parent *ExternalStack
	child  *InternalStack
}

// NewCombinedStack creates a new CombinedStack.
func NewCombinedStack(parent *ExternalStack, child *InternalStack) *CombinedStack {
	return &CombinedStack{
		parent: parent,
		child:  child,
	}
}

// Status returns the current status of the fixture stack.
func (s *CombinedStack) Status() Status {
	if ps := s.parent.Status(); ps != StatusGreen {
		return ps
	}
	return s.child.Status()
}

// Errors returns errors to be reported for tests depending on this fixture
// stack. See InternalStack.Errors for details.
func (s *CombinedStack) Errors() []*protocol.Error {
	if pe := s.parent.Errors(); len(pe) > 0 {
		return pe
	}
	return s.child.Errors()
}

// Val returns the fixture value obtained on setup.
func (s *CombinedStack) Val() interface{} {
	if len(s.child.stack) > 0 {
		return s.child.Val()
	}
	return s.parent.Val()
}

// Push adds a new fixture to the top of the fixture stack.
// See InternalStack.Push for details.
func (s *CombinedStack) Push(ctx context.Context, fixt *testing.FixtureInstance) error {
	return s.child.Push(ctx, fixt)
}

// Pop removes the top-most fixture from the fixture stack.
// See InternalStack.Pop for details.
func (s *CombinedStack) Pop(ctx context.Context) error {
	return s.child.Pop(ctx)
}

// Reset resets all fixtures on the stack if the stack is green.
// Reset clears the dirty flag of the stack.
// See InternalStack.Reset for details.
func (s *CombinedStack) Reset(ctx context.Context) error {
	if err := s.SetDirty(ctx, false); err != nil {
		return errors.Wrap(err, "stack reset")
	}
	switch s.Status() {
	case StatusGreen:
	case StatusRed:
		return nil
	case StatusYellow:
		return errors.New("BUG: Reset called for a yellow fixture stack")
	}

	if err := s.parent.Reset(ctx); err != nil {
		return err
	}
	switch s.parent.Status() {
	case StatusGreen:
	case StatusRed:
		return errors.New("BUG: parent fixture is red after calling Reset")
	case StatusYellow:
		return nil
	}

	if err := s.child.Reset(ctx); err != nil {
		return err
	}
	switch s.child.Status() {
	case StatusGreen:
	case StatusRed:
		return errors.New("BUG: child fixture is red after calling Reset")
	case StatusYellow:
		return nil
	}
	return nil
}

// PreTest runs PreTests on the fixtures.
// It returns a post test hook that runs PostTests on the fixtures.
func (s *CombinedStack) PreTest(ctx context.Context, test *protocol.Entity, outDir string, out testing.OutputStream, condition *testing.EntityCondition) (func(context.Context) error, error) {
	if s.Status() != StatusGreen {
		return nil, fmt.Errorf("BUG: PreTest called for a %v fixture", s.Status())
	}
	parentPostTest, err := s.parent.PreTest(ctx, test, condition)
	if err != nil {
		return nil, err
	}
	childPostTest, err := s.child.PreTest(ctx, outDir, out, condition)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		if s.Status() != StatusGreen {
			return fmt.Errorf("BUG: PreTest called for a %v fixture", s.Status())
		}
		if err := childPostTest(ctx); err != nil {
			return err
		}
		return parentPostTest(ctx)
	}, nil
}

// SetDirty marks the fixture stack dirty. It returns an error if dirty is true
// and the stack is already dirty.
// The dirty flag can be cleared by calling Reset. SetDirty(true) can be called
// before running a test to make sure Reset is called for sure between tests.
func (s *CombinedStack) SetDirty(ctx context.Context, dirty bool) error {
	if err := s.parent.SetDirty(ctx, dirty); err != nil {
		return err
	}
	if dirty {
		s.child.MarkDirty()
	} else {
		s.child.dirty = false
	}
	return nil
}

// Top returns the state of the top fixture.
// If the child stack is empty, it returns zero value.
func (s *CombinedStack) Top() *protocol.StartFixtureState {
	if len(s.child.stack) == 0 {
		return nil
	}
	f := s.child.top()
	return &protocol.StartFixtureState{
		Name:   f.Name(),
		Errors: f.Errors(),
	}
}
