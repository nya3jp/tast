// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"fmt"
	gotesting "testing"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/testing"
	"github.com/google/go-cmp/cmp"
)

type fakeFixture struct {
	setUpFunc    func(ctx context.Context, s *testing.FixtState) interface{}
	resetFunc    func(ctx context.Context) error
	tearDownFunc func(ctx context.Context, s *testing.FixtState)
}

type fakeFixtureOption func(ff *fakeFixture)

func withSetUp(f func(ctx context.Context, s *testing.FixtState) interface{}) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.setUpFunc = f
	}
}

func withReset(f func(ctx context.Context) error) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.resetFunc = f
	}
}

func withTearDown(f func(ctx context.Context, s *testing.FixtState)) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.tearDownFunc = f
	}
}

func newFakeFixture(opts ...fakeFixtureOption) *fakeFixture {
	ff := &fakeFixture{
		setUpFunc:    func(ctx context.Context, s *testing.FixtState) interface{} { return nil },
		resetFunc:    func(ctx context.Context) error { return nil },
		tearDownFunc: func(ctx context.Context, s *testing.FixtState) {},
	}
	for _, opt := range opts {
		opt(ff)
	}
	return ff
}

func (f *fakeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return f.setUpFunc(ctx, s)
}

func (f *fakeFixture) Reset(ctx context.Context) error {
	return f.resetFunc(ctx)
}

func (f *fakeFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	panic("not implemented")
}

func (f *fakeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	panic("not implemented")
}

func (f *fakeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.tearDownFunc(ctx, s)
}

func TestFixtureStackInitStatus(t *gotesting.T) {
	stack := newFixtureStack(&Config{}, newOutputSink())

	if got := stack.Status(); got != statusGreen {
		t.Fatalf("Initial status is %v; want %v", got, statusGreen)
	}
}

func TestFixtureStackStatusTransitionGreen(t *gotesting.T) {
	ctx := context.Background()
	stack := newFixtureStack(&Config{}, newOutputSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.Fixture{Impl: newFakeFixture()})
	}
	reset := func() error {
		return stack.Reset(ctx)
	}
	pop := func() error {
		return stack.Pop(ctx)
	}

	for i, step := range []struct {
		f    func() error
		want fixtureStatus
	}{
		{pushGreen, statusGreen},
		{pushGreen, statusGreen},
		{pushGreen, statusGreen},
		{reset, statusGreen},
		{pop, statusGreen},
		{pop, statusGreen},
		{pop, statusGreen},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if got := stack.Status(); got != step.want {
			t.Fatalf("Step %d: got %v, want %v", i, got, step.want)
		}
	}
}

func TestFixtureStackStatusTransitionRed(t *gotesting.T) {
	ctx := context.Background()
	stack := newFixtureStack(&Config{}, newOutputSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.Fixture{Impl: newFakeFixture()})
	}
	pushRed := func() error {
		return stack.Push(ctx, &testing.Fixture{
			Impl: newFakeFixture(withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Failed")
				return nil
			})),
		})
	}
	pop := func() error {
		return stack.Pop(ctx)
	}

	for i, step := range []struct {
		f    func() error
		want fixtureStatus
	}{
		{pushGreen, statusGreen},
		{pushGreen, statusGreen},
		{pushGreen, statusGreen},
		{pushRed, statusRed},
		{pushGreen, statusRed},
		{pushGreen, statusRed},
		{pushGreen, statusRed},
		{pop, statusRed},
		{pop, statusRed},
		{pop, statusRed},
		{pop, statusGreen},
		{pop, statusGreen},
		{pop, statusGreen},
		{pop, statusGreen},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if got := stack.Status(); got != step.want {
			t.Fatalf("Step %d: got %v, want %v", i, got, step.want)
		}
	}
}

func TestFixtureStackStatusTransitionYellow(t *gotesting.T) {
	ctx := context.Background()
	stack := newFixtureStack(&Config{}, newOutputSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.Fixture{Impl: newFakeFixture()})
	}
	pushYellow := func() error {
		return stack.Push(ctx, &testing.Fixture{
			Impl: newFakeFixture(withReset(func(ctx context.Context) error {
				return errors.New("failed")
			})),
		})
	}
	reset := func() error {
		return stack.Reset(ctx)
	}
	pop := func() error {
		return stack.Pop(ctx)
	}

	for i, step := range []struct {
		f    func() error
		want fixtureStatus
	}{
		{pushGreen, statusGreen},
		{pushYellow, statusGreen},
		{pushGreen, statusGreen},
		{reset, statusYellow},
		{pop, statusYellow},
		{pop, statusGreen},
		{pop, statusGreen},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if got := stack.Status(); got != step.want {
			t.Fatalf("Step %d: got %v, want %v", i, got, step.want)
		}
	}
}

func TestFixtureStackContext(t *gotesting.T) {
	serviceDeps := []string{"svc1", "svc2"}

	ctx := context.Background()
	stack := newFixtureStack(&Config{}, newOutputSink())

	verifyContext := func(t *gotesting.T, ctx context.Context) {
		t.Helper()
		if svcs, ok := testing.ContextServiceDeps(ctx); !ok {
			t.Error("ServiceDeps not available")
		} else if diff := cmp.Diff(svcs, serviceDeps); diff != "" {
			t.Errorf("ServiceDeps mismatch (-got +want):\n%s", diff)
		}
	}

	ff := newFakeFixture(
		withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			t.Run("SetUp", func(t *gotesting.T) {
				verifyContext(t, ctx)
				verifyContext(t, s.FixtContext())
			})
			return nil
		}),
		withReset(func(ctx context.Context) error {
			t.Run("Reset", func(t *gotesting.T) {
				verifyContext(t, ctx)
			})
			return nil
		}),
		withTearDown(func(ctx context.Context, s *testing.FixtState) {
			t.Run("TearDown", func(t *gotesting.T) {
				verifyContext(t, ctx)
				verifyContext(t, s.FixtContext())
			})
		}))

	fixt := &testing.Fixture{
		Impl:        ff,
		ServiceDeps: serviceDeps,
	}

	if err := stack.Push(ctx, fixt); err != nil {
		t.Fatal("Push: ", err)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop: ", err)
	}
}

func TestFixtureStackState(t *gotesting.T) {
	const localBundleDir = "/path/to/local/bundles"

	ctx := context.Background()
	cfg := &Config{
		RemoteData: &testing.RemoteData{
			RPCHint: &testing.RPCHint{
				LocalBundleDir: localBundleDir,
			},
		},
	}
	stack := newFixtureStack(cfg, newOutputSink())

	verifyState := func(t *gotesting.T, s *testing.FixtState) {
		if dir := s.RPCHint().LocalBundleDir; dir != localBundleDir {
			t.Errorf("RPCHint.LocalBundleDir = %q; want %q", dir, localBundleDir)
		}
	}

	ff := newFakeFixture(
		withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			t.Run("SetUp", func(t *gotesting.T) {
				verifyState(t, s)
			})
			return nil
		}),
		withTearDown(func(ctx context.Context, s *testing.FixtState) {
			t.Run("TearDown", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}))

	fixt := &testing.Fixture{
		Impl: ff,
	}

	if err := stack.Push(ctx, fixt); err != nil {
		t.Fatal("Push: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop: ", err)
	}
}

func TestFixtureStackOutput(t *gotesting.T) {
	ctx := context.Background()
	sink := newOutputSink()
	stack := newFixtureStack(&Config{}, sink)

	newLoggingFixture := func(id int) *testing.Fixture {
		return &testing.Fixture{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: newFakeFixture(
				withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					logging.ContextLogf(ctx, "SetUp %d via Context", id)
					logging.ContextLogf(s.FixtContext(), "SetUp %d via Fixture-scoped Context", id)
					s.Logf("SetUp %d via State", id)
					return nil
				}),
				withReset(func(ctx context.Context) error {
					logging.ContextLogf(ctx, "Reset %d via Context", id)
					return nil
				}),
				withTearDown(func(ctx context.Context, s *testing.FixtState) {
					logging.ContextLogf(ctx, "TearDown %d via Context", id)
					logging.ContextLogf(s.FixtContext(), "TearDown %d via Fixture-scoped Context", id)
					s.Logf("TearDown %d via State", id)
				})),
		}
	}

	if err := stack.Push(ctx, newLoggingFixture(1)); err != nil {
		t.Fatal("Push 1: ", err)
	}
	if err := stack.Push(ctx, newLoggingFixture(2)); err != nil {
		t.Fatal("Push 2: ", err)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 2: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 1: ", err)
	}

	msgs, err := sink.ReadAll()
	if err != nil {
		t.Fatal("ReadAll: ", err)
	}

	want := []control.Msg{
		&control.EntityStart{Info: testing.EntityInfo{
			Name: "fixt1",
			Type: testing.EntityFixture,
		}},
		&control.EntityLog{Text: "SetUp 1 via Context"},
		&control.EntityLog{Text: "SetUp 1 via Fixture-scoped Context"},
		&control.EntityLog{Text: "SetUp 1 via State"},
		&control.EntityStart{Info: testing.EntityInfo{
			Name: "fixt2",
			Type: testing.EntityFixture,
		}},
		&control.EntityLog{Text: "SetUp 2 via Context"},
		&control.EntityLog{Text: "SetUp 2 via Fixture-scoped Context"},
		&control.EntityLog{Text: "SetUp 2 via State"},
		&control.EntityLog{Text: "Reset 1 via Context"},
		&control.EntityLog{Text: "Reset 2 via Context"},
		&control.EntityLog{Text: "TearDown 2 via Context"},
		&control.EntityLog{Text: "TearDown 2 via Fixture-scoped Context"},
		&control.EntityLog{Text: "TearDown 2 via State"},
		&control.EntityEnd{Name: "fixt2"},
		&control.EntityLog{Text: "TearDown 1 via Context"},
		&control.EntityLog{Text: "TearDown 1 via Fixture-scoped Context"},
		&control.EntityLog{Text: "TearDown 1 via State"},
		&control.EntityEnd{Name: "fixt1"},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

// TODO: Write a test of skipping SetUp on pushing onto a red stack
