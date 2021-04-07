// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

// fakeFixture is a customizable implementation of testing.FixtureImpl.
type fakeFixture struct {
	setUpFunc    func(ctx context.Context, s *testing.FixtState) interface{}
	resetFunc    func(ctx context.Context) error
	preTestFunc  func(ctx context.Context, s *testing.FixtTestState)
	postTestFunc func(ctx context.Context, s *testing.FixtTestState)
	tearDownFunc func(ctx context.Context, s *testing.FixtState)
}

// fakeFixtureOption is an option passed to newFakeFixture to customize
// a constructed fakeFixture.
type fakeFixtureOption func(ff *fakeFixture)

// withSetUp returns an option to set a function called back on SetUp.
func withSetUp(f func(ctx context.Context, s *testing.FixtState) interface{}) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.setUpFunc = f
	}
}

// withReset returns an option to set a function called back on Reset.
func withReset(f func(ctx context.Context) error) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.resetFunc = f
	}
}

// withPreTest returns an option to set a function called back on PreTest.
func withPreTest(f func(ctx context.Context, s *testing.FixtTestState)) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.preTestFunc = f
	}
}

// withPostTest returns an option to set a function called back on PostTest.
func withPostTest(f func(ctx context.Context, s *testing.FixtTestState)) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.postTestFunc = f
	}
}

// withTearDown returns an option to set a function called back on TearDown.
func withTearDown(f func(ctx context.Context, s *testing.FixtState)) fakeFixtureOption {
	return func(ff *fakeFixture) {
		ff.tearDownFunc = f
	}
}

// newFakeFixture creates a new fake fixture.
func newFakeFixture(opts ...fakeFixtureOption) *fakeFixture {
	ff := &fakeFixture{
		setUpFunc:    func(ctx context.Context, s *testing.FixtState) interface{} { return nil },
		resetFunc:    func(ctx context.Context) error { return nil },
		preTestFunc:  func(ctx context.Context, s *testing.FixtTestState) {},
		postTestFunc: func(ctx context.Context, s *testing.FixtTestState) {},
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
	f.preTestFunc(ctx, s)
}

func (f *fakeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	f.postTestFunc(ctx, s)
}

func (f *fakeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.tearDownFunc(ctx, s)
}

// TestFixtureStackInitStatus checks the initial status of a fixture stack.
func TestFixtureStackInitStatus(t *gotesting.T) {
	stack := NewFixtureStack(&Config{}, newOutputSink())

	if got := stack.Status(); got != statusGreen {
		t.Fatalf("Initial status is %v; want %v", got, statusGreen)
	}
}

// TestFixtureStackStatusTransitionGreen tests status transition of a fixture
// stack on pushing healthy fixtures.
func TestFixtureStackStatusTransitionGreen(t *gotesting.T) {
	ctx := context.Background()
	stack := NewFixtureStack(&Config{}, newOutputSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{Impl: newFakeFixture()})
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

// TestFixtureStackStatusTransitionRed tests status transition of a fixture
// stack on pushing a fixture that fails to set up.
func TestFixtureStackStatusTransitionRed(t *gotesting.T) {
	ctx := context.Background()
	stack := NewFixtureStack(&Config{}, newOutputSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{Impl: newFakeFixture()})
	}
	pushRed := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{
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

// TestFixtureStackStatusTransitionYellow tests status transition of a fixture
// stack on pushing a fixture that fails to reset.
func TestFixtureStackStatusTransitionYellow(t *gotesting.T) {
	ctx := context.Background()
	stack := NewFixtureStack(&Config{}, newOutputSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{Impl: newFakeFixture()})
	}
	pushYellow := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{
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

// TestFixtureStackMarkDirty tests dirtiness check of fixture stacks.
func TestFixtureStackMarkDirty(t *gotesting.T) {
	ctx := context.Background()
	stack := NewFixtureStack(&Config{}, newOutputSink())

	if err := stack.MarkDirty(); err != nil {
		t.Errorf("MarkDirty failed for initial stack: %v", err)
	}
	if err := stack.MarkDirty(); err == nil {
		t.Error("MarkDirty unexpectedly succeeded for a dirty stack")
	}

	if err := stack.Reset(ctx); err != nil {
		t.Errorf("Reset failed: %v", err)
	}

	if err := stack.MarkDirty(); err != nil {
		t.Errorf("MarkDirty failed for reset stack: %v", err)
	}
	if err := stack.MarkDirty(); err == nil {
		t.Error("MarkDirty unexpectedly succeeded for a dirty stack")
	}
}

// TestFixtureStackContext checks context.Context passed to fixture methods.
func TestFixtureStackContext(t *gotesting.T) {
	const fixtureName = "fixt"
	serviceDeps := []string{"svc1", "svc2"}

	ctx := context.Background()

	baseOutDir := testutil.TempDir(t)
	defer os.RemoveAll(baseOutDir)

	stack := NewFixtureStack(&Config{OutDir: baseOutDir}, newOutputSink())

	fixtureOutDir := filepath.Join(baseOutDir, fixtureName)
	testOutDir := filepath.Join(baseOutDir, "pkg.Test")

	verifyContext := func(t *gotesting.T, ctx context.Context, wantOutDir string) {
		t.Helper()
		if svcs, ok := testcontext.ServiceDeps(ctx); !ok {
			t.Error("ServiceDeps not available")
		} else if diff := cmp.Diff(svcs, serviceDeps); diff != "" {
			t.Errorf("ServiceDeps mismatch (-got +want):\n%s", diff)
		}
		if _, ok := testcontext.SoftwareDeps(ctx); ok {
			t.Error("SoftwareDeps unexpectedly available")
		}
		outDir, ok := testcontext.OutDir(ctx)
		if !ok {
			t.Error("OutDir not available")
		} else if outDir != wantOutDir {
			t.Errorf("OutDir = %q; want %q", outDir, wantOutDir)
		}
	}

	ff := newFakeFixture(
		withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			t.Run("SetUp", func(t *gotesting.T) {
				verifyContext(t, ctx, fixtureOutDir)
				verifyContext(t, s.FixtContext(), fixtureOutDir)
			})
			return nil
		}),
		withReset(func(ctx context.Context) error {
			t.Run("Reset", func(t *gotesting.T) {
				verifyContext(t, ctx, fixtureOutDir)
			})
			return nil
		}),
		withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PreTest", func(t *gotesting.T) {
				verifyContext(t, ctx, testOutDir)
				verifyContext(t, s.TestContext(), testOutDir)
			})
		}),
		withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PostTest", func(t *gotesting.T) {
				verifyContext(t, ctx, testOutDir)
				verifyContext(t, s.TestContext(), testOutDir)
			})
		}),
		withTearDown(func(ctx context.Context, s *testing.FixtState) {
			t.Run("TearDown", func(t *gotesting.T) {
				verifyContext(t, ctx, fixtureOutDir)
				verifyContext(t, s.FixtContext(), fixtureOutDir)
			})
		}))

	fixt := &testing.FixtureInstance{
		Name:        fixtureName,
		Impl:        ff,
		ServiceDeps: serviceDeps,
	}

	troot := testing.NewTestEntityRoot(&testing.TestInstance{}, &testing.RuntimeConfig{OutDir: testOutDir}, nil)

	if err := stack.Push(ctx, fixt); err != nil {
		t.Fatal("Push: ", err)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if err := stack.PreTest(ctx, troot); err != nil {
		t.Fatal("PreTest: ", err)
	}
	if err := stack.PostTest(ctx, troot); err != nil {
		t.Fatal("PostTest: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop: ", err)
	}
}

// TestFixtureStackState checks state objects passed to fixture methods.
func TestFixtureStackState(t *gotesting.T) {
	const localBundleDir = "/path/to/local/bundles"

	rd := &testing.RemoteData{
		RPCHint: testing.NewRPCHint(localBundleDir, nil),
	}

	ctx := context.Background()
	stack := NewFixtureStack(&Config{RemoteData: rd}, newOutputSink())

	type stateLike interface {
		RPCHint() *testing.RPCHint
	}
	verifyState := func(t *gotesting.T, s stateLike) {
		if dir := testing.ExtractLocalBundleDir(s.RPCHint()); dir != localBundleDir {
			t.Errorf("localBundleDir of RPCHint = %q; want %q", dir, localBundleDir)
		}
	}

	ff := newFakeFixture(
		withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			t.Run("SetUp", func(t *gotesting.T) {
				verifyState(t, s)
			})
			return nil
		}),
		withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PreTest", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}),
		withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PostTest", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}),
		withTearDown(func(ctx context.Context, s *testing.FixtState) {
			t.Run("TearDown", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}))

	fixt := &testing.FixtureInstance{
		Impl: ff,
	}

	troot := testing.NewTestEntityRoot(&testing.TestInstance{}, &testing.RuntimeConfig{RemoteData: rd}, nil)

	if err := stack.Push(ctx, fixt); err != nil {
		t.Fatal("Push: ", err)
	}
	if err := stack.PreTest(ctx, troot); err != nil {
		t.Fatal("PreTest: ", err)
	}
	if err := stack.PostTest(ctx, troot); err != nil {
		t.Fatal("PostTest: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop: ", err)
	}
}

// TestFixtureStackVal tests that fixture values are passed around correctly.
func TestFixtureStackVal(t *gotesting.T) {
	const (
		val1 = "val1"
		val2 = "val2"
	)

	ctx := context.Background()
	stack := NewFixtureStack(&Config{}, newOutputSink())

	if val := stack.Val(); val != nil {
		t.Errorf("Init: Val() = %v; want nil", val)
	}

	// Push fixtures and check values.
	if err := stack.Push(ctx, &testing.FixtureInstance{
		Impl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				if val := s.ParentValue(); val != nil {
					t.Errorf("SetUp 1: ParentValue() = %v; want nil", val)
				}
				return val1
			}))}); err != nil {
		t.Fatal("Push 1: ", err)
	}
	if val := stack.Val(); val != val1 {
		t.Errorf("After Push 1: Val() = %v; want %v", val, val1)
	}

	if err := stack.Push(ctx, &testing.FixtureInstance{
		Impl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				if val := s.ParentValue(); val != val1 {
					t.Errorf("SetUp 2: ParentValue() = %v; want %v", val, val1)
				}
				return val2
			}),
			withReset(func(ctx context.Context) error {
				return errors.New("failure")
			}))}); err != nil {
		t.Fatal("Push 2: ", err)
	}
	if val := stack.Val(); val != val2 {
		t.Errorf("After Push 2: Val() = %v; want %v", val, val2)
	}

	// Call Reset. Even if the stack is yellow, Val still succeeds.
	if s := stack.Status(); s != statusGreen {
		t.Errorf("After Push 2: Status() = %v; want %v", s, statusGreen)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if s := stack.Status(); s != statusYellow {
		t.Errorf("After Reset: Status() = %v; want %v", s, statusYellow)
	}
	if val := stack.Val(); val != val2 {
		t.Errorf("After Reset: Val() = %v; want %v", val, val2)
	}

	// Pop fixtures.
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 2: ", err)
	}
	if val := stack.Val(); val != val1 {
		t.Errorf("After Pop 2: Val() = %v; want %v", val, val1)
	}

	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 1: ", err)
	}
	if val := stack.Val(); val != nil {
		t.Errorf("After Pop 1: Val() = %v; want nil", val)
	}
}

// TestFixtureStackErrors tests Errors method.
func TestFixtureStackErrors(t *gotesting.T) {
	ctx := context.Background()
	stack := NewFixtureStack(&Config{}, newOutputSink())

	id := 0
	pushGreen := func() error {
		name := fmt.Sprintf("fixt.Green%d", id)
		id++
		return stack.Push(ctx, &testing.FixtureInstance{
			Name: name,
			Impl: newFakeFixture(),
		})
	}
	pushRed := func() error {
		name := fmt.Sprintf("fixt.Red%d", id)
		id++
		return stack.Push(ctx, &testing.FixtureInstance{
			Name: name,
			Impl: newFakeFixture(withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Setup failure 1")
				s.Error("Setup failure 2")
				return nil
			})),
		})
	}
	pop := func() error {
		return stack.Pop(ctx)
	}

	wantErrs := []*protocol.Error{
		{Reason: "[Fixture failure] fixt.Red1: Setup failure 1"},
		{Reason: "[Fixture failure] fixt.Red1: Setup failure 2"},
	}

	for i, step := range []struct {
		f    func() error
		want []*protocol.Error
	}{
		{pushGreen, nil},
		{pushRed, wantErrs},
		{pushRed, wantErrs},
		{pop, wantErrs},
		{pop, nil},
		{pop, nil},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		got := stack.Errors()
		if diff := cmp.Diff(got, step.want, cmpopts.IgnoreFields(protocol.Error{}, "Location")); diff != "" {
			t.Fatalf("Step %d: Errors mismatch (-got +want):\n%s", i, diff)
		}
	}
}

// TestFixtureStackOutputGreen tests control message outputs when all fixtures
// are healthy.
func TestFixtureStackOutputGreen(t *gotesting.T) {
	ctx := context.Background()
	sink := newOutputSink()
	ti := &testing.TestInstance{Name: "pkg.Test"}
	troot := testing.NewTestEntityRoot(ti, &testing.RuntimeConfig{}, newEntityOutputStream(sink, ti.EntityProto()))
	stack := NewFixtureStack(&Config{}, sink)

	newLoggingFixture := func(id int) *testing.FixtureInstance {
		return &testing.FixtureInstance{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: newFakeFixture(
				withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					testcontext.Logf(ctx, "SetUp %d via Context", id)
					testcontext.Logf(s.FixtContext(), "SetUp %d via Fixture-scoped Context", id)
					s.Logf("SetUp %d via State", id)
					return nil
				}),
				withReset(func(ctx context.Context) error {
					testcontext.Logf(ctx, "Reset %d via Context", id)
					return nil
				}),
				withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
					testcontext.Logf(ctx, "PreTest %d via Context", id)
					s.Logf("PreTest %d via State", id)
				}),
				withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
					testcontext.Logf(ctx, "PostTest %d via Context", id)
					s.Logf("PostTest %d via State", id)
				}),
				withTearDown(func(ctx context.Context, s *testing.FixtState) {
					testcontext.Logf(ctx, "TearDown %d via Context", id)
					testcontext.Logf(s.FixtContext(), "TearDown %d via Fixture-scoped Context", id)
					s.Logf("TearDown %d via State", id)
				})),
		}
	}
	fixt1 := newLoggingFixture(1)
	fixt2 := newLoggingFixture(2)

	if err := stack.Push(ctx, fixt1); err != nil {
		t.Fatal("Push 1: ", err)
	}
	if err := stack.Push(ctx, fixt2); err != nil {
		t.Fatal("Push 2: ", err)
	}
	if err := stack.PreTest(ctx, troot); err != nil {
		t.Fatal("PreTest: ", err)
	}
	if err := stack.PostTest(ctx, troot); err != nil {
		t.Fatal("PostTest: ", err)
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

	msgs := sink.ReadAll()

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "SetUp 1 via Context"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "SetUp 1 via Fixture-scoped Context"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "SetUp 1 via State"},
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "SetUp 2 via Context"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "SetUp 2 via Fixture-scoped Context"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "SetUp 2 via State"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PreTest 1 via Context"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PreTest 1 via State"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PreTest 2 via Context"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PreTest 2 via State"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PostTest 2 via Context"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PostTest 2 via State"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PostTest 1 via Context"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "PostTest 1 via State"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "Reset 1 via Context"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "Reset 2 via Context"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "TearDown 2 via Context"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "TearDown 2 via Fixture-scoped Context"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "TearDown 2 via State"},
		&protocol.EntityEndEvent{EntityName: "fixt2"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "TearDown 1 via Context"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "TearDown 1 via Fixture-scoped Context"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "TearDown 1 via State"},
		&protocol.EntityEndEvent{EntityName: "fixt1"},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

// TestFixtureStackOutputRed tests control message outputs when a fixture fails
// to set up.
func TestFixtureStackOutputRed(t *gotesting.T) {
	ctx := context.Background()
	sink := newOutputSink()
	stack := NewFixtureStack(&Config{}, sink)

	newLoggingFixture := func(id int, setUp bool) *testing.FixtureInstance {
		return &testing.FixtureInstance{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: newFakeFixture(
				withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					testcontext.Logf(ctx, "SetUp %d", id)
					if !setUp {
						s.Errorf("SetUp %d failure", id)
					}
					return nil
				}),
				withReset(func(ctx context.Context) error {
					testcontext.Logf(ctx, "Reset %d", id)
					return nil
				}),
				withTearDown(func(ctx context.Context, s *testing.FixtState) {
					testcontext.Logf(ctx, "TearDown %d", id)
				})),
		}
	}
	fixt1 := newLoggingFixture(1, true)
	fixt2 := newLoggingFixture(2, false)
	fixt3 := newLoggingFixture(3, true)

	// Push and pop three fixtures. Second fixture fails to set up.
	if err := stack.Push(ctx, fixt1); err != nil {
		t.Fatal("Push 1: ", err)
	}
	if err := stack.Push(ctx, fixt2); err != nil {
		t.Fatal("Push 2: ", err)
	}
	if err := stack.Push(ctx, fixt3); err != nil {
		t.Fatal("Push 3: ", err)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 3: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 2: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 1: ", err)
	}

	msgs := sink.ReadAll()

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "SetUp 1"},
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "SetUp 2"},
		&protocol.EntityErrorEvent{EntityName: "fixt2", Error: &protocol.Error{Reason: "SetUp 2 failure"}},
		&protocol.EntityEndEvent{EntityName: "fixt2"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "TearDown 1"},
		&protocol.EntityEndEvent{EntityName: "fixt1"},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

// TestFixtureStackOutputYellow tests control message outputs when a fixture
// fails to reset.
func TestFixtureStackOutputYellow(t *gotesting.T) {
	ctx := context.Background()
	sink := newOutputSink()
	stack := NewFixtureStack(&Config{}, sink)

	newLoggingFixture := func(id int, reset bool) *testing.FixtureInstance {
		return &testing.FixtureInstance{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: newFakeFixture(
				withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					testcontext.Logf(ctx, "SetUp %d", id)
					return nil
				}),
				withReset(func(ctx context.Context) error {
					testcontext.Logf(ctx, "Reset %d", id)
					if !reset {
						return errors.New("failure")
					}
					return nil
				}),
				withTearDown(func(ctx context.Context, s *testing.FixtState) {
					testcontext.Logf(ctx, "TearDown %d", id)
				})),
		}
	}
	fixt1 := newLoggingFixture(1, true)
	fixt2 := newLoggingFixture(2, false)
	fixt3 := newLoggingFixture(3, true)

	// Push and pop three fixtures. Second fixture fails to reset.
	if err := stack.Push(ctx, fixt1); err != nil {
		t.Fatal("Push 1: ", err)
	}
	if err := stack.Push(ctx, fixt2); err != nil {
		t.Fatal("Push 2: ", err)
	}
	if err := stack.Push(ctx, fixt3); err != nil {
		t.Fatal("Push 3: ", err)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 3: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 2: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop 1: ", err)
	}

	msgs := sink.ReadAll()

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "SetUp 1"},
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "SetUp 2"},
		&protocol.EntityStartEvent{Entity: fixt3.EntityProto()},
		&protocol.EntityLogEvent{EntityName: "fixt3", Text: "SetUp 3"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "Reset 2"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "Fixture failed to reset: failure; recovering"},
		&protocol.EntityLogEvent{EntityName: "fixt3", Text: "TearDown 3"},
		&protocol.EntityEndEvent{EntityName: "fixt3"},
		&protocol.EntityLogEvent{EntityName: "fixt2", Text: "TearDown 2"},
		&protocol.EntityEndEvent{EntityName: "fixt2"},
		&protocol.EntityLogEvent{EntityName: "fixt1", Text: "TearDown 1"},
		&protocol.EntityEndEvent{EntityName: "fixt1"},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}
