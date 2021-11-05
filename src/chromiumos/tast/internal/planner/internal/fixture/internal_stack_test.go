// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/planner/internal/fixture"
	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/planner/internal/output/outputtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/testing/testfixture"
	"chromiumos/tast/testutil"
)

// TestInternalStackInitStatus checks the initial status of a fixture stack.
func TestInternalStackInitStatus(t *gotesting.T) {
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	if got := stack.Status(); got != fixture.StatusGreen {
		t.Fatalf("Initial status is %v; want %v", got, fixture.StatusGreen)
	}
}

// TestInternalStackStatusTransitionGreen tests status transition of a fixture
// stack on pushing healthy fixture.
func TestInternalStackStatusTransitionGreen(t *gotesting.T) {
	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{Impl: testfixture.New()})
	}
	reset := func() error {
		return stack.Reset(ctx)
	}
	pop := func() error {
		return stack.Pop(ctx)
	}

	for i, step := range []struct {
		f    func() error
		want fixture.Status
	}{
		{pushGreen, fixture.StatusGreen},
		{pushGreen, fixture.StatusGreen},
		{pushGreen, fixture.StatusGreen},
		{reset, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if got := stack.Status(); got != step.want {
			t.Fatalf("Step %d: got %v, want %v", i, got, step.want)
		}
	}
}

// TestInternalStackStatusTransitionRed tests status transition of a fixture
// stack on pushing a fixture that fails to set up.
func TestInternalStackStatusTransitionRed(t *gotesting.T) {
	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{Impl: testfixture.New()})
	}
	pushRed := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{
			Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
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
		want fixture.Status
	}{
		{pushGreen, fixture.StatusGreen},
		{pushGreen, fixture.StatusGreen},
		{pushGreen, fixture.StatusGreen},
		{pushRed, fixture.StatusRed},
		{pushGreen, fixture.StatusRed},
		{pushGreen, fixture.StatusRed},
		{pushGreen, fixture.StatusRed},
		{pop, fixture.StatusRed},
		{pop, fixture.StatusRed},
		{pop, fixture.StatusRed},
		{pop, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if got := stack.Status(); got != step.want {
			t.Fatalf("Step %d: got %v, want %v", i, got, step.want)
		}
	}
}

// TestInternalStackStatusTransitionYellow tests status transition of a fixture
// stack on pushing a fixture that fails to reset.
func TestInternalStackStatusTransitionYellow(t *gotesting.T) {
	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	pushGreen := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{Impl: testfixture.New()})
	}
	pushYellow := func() error {
		return stack.Push(ctx, &testing.FixtureInstance{
			Impl: testfixture.New(testfixture.WithReset(func(ctx context.Context) error {
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
		want fixture.Status
	}{
		{pushGreen, fixture.StatusGreen},
		{pushYellow, fixture.StatusGreen},
		{pushGreen, fixture.StatusGreen},
		{reset, fixture.StatusYellow},
		{pop, fixture.StatusYellow},
		{pop, fixture.StatusGreen},
		{pop, fixture.StatusGreen},
	} {
		if err := step.f(); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if got := stack.Status(); got != step.want {
			t.Fatalf("Step %d: got %v, want %v", i, got, step.want)
		}
	}
}

// TestInternalStackMarkDirty tests dirtiness check of fixture stacks.
func TestInternalStackMarkDirty(t *gotesting.T) {
	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

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

// TestInternalStackContext checks context.Context passed to fixture methods.
func TestInternalStackContext(t *gotesting.T) {
	const fixtureName = "fixt"
	serviceDeps := []string{"svc1", "svc2"}

	ctx := context.Background()

	baseOutDir := testutil.TempDir(t)
	defer os.RemoveAll(baseOutDir)

	stack := fixture.NewInternalStack(&fixture.Config{OutDir: baseOutDir, GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

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

	ff := testfixture.New(
		testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			t.Run("SetUp", func(t *gotesting.T) {
				verifyContext(t, ctx, fixtureOutDir)
				verifyContext(t, s.FixtContext(), fixtureOutDir)
			})
			return nil
		}),
		testfixture.WithReset(func(ctx context.Context) error {
			t.Run("Reset", func(t *gotesting.T) {
				verifyContext(t, ctx, fixtureOutDir)
			})
			return nil
		}),
		testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PreTest", func(t *gotesting.T) {
				verifyContext(t, ctx, testOutDir)
				verifyContext(t, s.TestContext(), testOutDir)
			})
		}),
		testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PostTest", func(t *gotesting.T) {
				verifyContext(t, ctx, testOutDir)
				verifyContext(t, s.TestContext(), testOutDir)
			})
		}),
		testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
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

	if err := stack.Push(ctx, fixt); err != nil {
		t.Fatal("Push: ", err)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	postTest, err := stack.PreTest(ctx, testOutDir, nil, testing.NewEntityCondition())
	if err != nil {
		t.Fatal("PreTest: ", err)
	}
	if err := postTest(ctx); err != nil {
		t.Fatal("PostTest: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop: ", err)
	}
}

// TestInternalStackState checks state objects passed to fixture methods.
func TestInternalStackState(t *gotesting.T) {
	const localBundleDir = "/path/to/local/bundles"

	rd := &testing.RemoteData{
		RPCHint: testing.NewRPCHint(localBundleDir, nil),
	}

	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{RemoteData: rd, GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	type stateLike interface {
		RPCHint() *testing.RPCHint
	}
	verifyState := func(t *gotesting.T, s stateLike) {
		if dir := testing.ExtractLocalBundleDir(s.RPCHint()); dir != localBundleDir {
			t.Errorf("localBundleDir of RPCHint = %q; want %q", dir, localBundleDir)
		}
	}

	ff := testfixture.New(
		testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			t.Run("SetUp", func(t *gotesting.T) {
				verifyState(t, s)
			})
			return nil
		}),
		testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PreTest", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}),
		testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
			t.Run("PostTest", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}),
		testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
			t.Run("TearDown", func(t *gotesting.T) {
				verifyState(t, s)
			})
		}))

	fixt := &testing.FixtureInstance{
		Impl: ff,
	}

	if err := stack.Push(ctx, fixt); err != nil {
		t.Fatal("Push: ", err)
	}
	postTest, err := stack.PreTest(ctx, "", nil, testing.NewEntityCondition())
	if err != nil {
		t.Fatal("PreTest: ", err)
	}
	if err := postTest(ctx); err != nil {
		t.Fatal("PostTest: ", err)
	}
	if err := stack.Pop(ctx); err != nil {
		t.Fatal("Pop: ", err)
	}
}

// TestInternalStackVal tests that fixture values are passed around correctly.
func TestInternalStackVal(t *gotesting.T) {
	const (
		val1 = "val1"
		val2 = "val2"
	)

	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	if val := stack.Val(); val != nil {
		t.Errorf("Init: Val() = %v; want nil", val)
	}

	// Push fixtures and check values.
	if err := stack.Push(ctx, &testing.FixtureInstance{
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
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
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				if val := s.ParentValue(); val != val1 {
					t.Errorf("SetUp 2: ParentValue() = %v; want %v", val, val1)
				}
				return val2
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				return errors.New("failure")
			}))}); err != nil {
		t.Fatal("Push 2: ", err)
	}
	if val := stack.Val(); val != val2 {
		t.Errorf("After Push 2: Val() = %v; want %v", val, val2)
	}

	// Call Reset. Even if the stack is yellow, Val still succeeds.
	if s := stack.Status(); s != fixture.StatusGreen {
		t.Errorf("After Push 2: fixture.Status() = %v; want %v", s, fixture.StatusGreen)
	}
	if err := stack.Reset(ctx); err != nil {
		t.Fatal("Reset: ", err)
	}
	if s := stack.Status(); s != fixture.StatusYellow {
		t.Errorf("After Reset: fixture.Status() = %v; want %v", s, fixture.StatusYellow)
	}
	if val := stack.Val(); val != val2 {
		t.Errorf("After Reset: Val() = %v; want %v", val, val2)
	}

	// Pop fixture.
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

// TestInternalStackErrors tests Errors method.
func TestInternalStackErrors(t *gotesting.T) {
	ctx := context.Background()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, outputtest.NewSink())

	id := 0
	pushGreen := func() error {
		name := fmt.Sprintf("fixt.Green%d", id)
		id++
		return stack.Push(ctx, &testing.FixtureInstance{
			Name: name,
			Impl: testfixture.New(),
		})
	}
	pushRed := func() error {
		name := fmt.Sprintf("fixt.Red%d", id)
		id++
		return stack.Push(ctx, &testing.FixtureInstance{
			Name: name,
			Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
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
		if diff := cmp.Diff(got, step.want, protocmp.Transform(), protocmp.IgnoreFields(&protocol.Error{}, "location")); diff != "" {
			t.Fatalf("Step %d: Errors mismatch (-got +want):\n%s", i, diff)
		}
	}
}

// TestInternalStackOutputGreen tests control message outputs when all fixtures
// are healthy.
func TestInternalStackOutputGreen(t *gotesting.T) {
	ctx := context.Background()
	sink := outputtest.NewSink()
	ti := &testing.TestInstance{Name: "pkg.Test"}

	out := output.NewEntityStream(sink, ti.EntityProto())
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, sink)

	newLoggingFixture := func(id int) *testing.FixtureInstance {
		return &testing.FixtureInstance{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: testfixture.New(
				testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					logging.Infof(ctx, "SetUp %d via Context", id)
					logging.Infof(s.FixtContext(), "SetUp %d via Fixture-scoped Context", id)
					s.Logf("SetUp %d via State", id)
					return nil
				}),
				testfixture.WithReset(func(ctx context.Context) error {
					logging.Infof(ctx, "Reset %d via Context", id)
					return nil
				}),
				testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
					logging.Infof(ctx, "PreTest %d via Context", id)
					s.Logf("PreTest %d via State", id)
				}),
				testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
					logging.Infof(ctx, "PostTest %d via Context", id)
					s.Logf("PostTest %d via State", id)
				}),
				testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
					logging.Infof(ctx, "TearDown %d via Context", id)
					logging.Infof(s.FixtContext(), "TearDown %d via Fixture-scoped Context", id)
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
	postTest, err := stack.PreTest(ctx, "", out, testing.NewEntityCondition())
	if err != nil {
		t.Fatal("PreTest: ", err)
	}
	if err := postTest(ctx); err != nil {
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
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

// TestInternalStackOutputRed tests control message outputs when a fixture fails
// to set up.
func TestInternalStackOutputRed(t *gotesting.T) {
	ctx := context.Background()
	sink := outputtest.NewSink()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, sink)

	newLoggingFixture := func(id int, setUp bool) *testing.FixtureInstance {
		return &testing.FixtureInstance{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: testfixture.New(
				testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					logging.Infof(ctx, "SetUp %d", id)
					if !setUp {
						s.Errorf("SetUp %d failure", id)
					}
					return nil
				}),
				testfixture.WithReset(func(ctx context.Context) error {
					logging.Infof(ctx, "Reset %d", id)
					return nil
				}),
				testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
					logging.Infof(ctx, "TearDown %d", id)
				})),
		}
	}
	fixt1 := newLoggingFixture(1, true)
	fixt2 := newLoggingFixture(2, false)
	fixt3 := newLoggingFixture(3, true)

	// Push and pop three fixture. Second fixture fails to set up.
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
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

// TestInternalStackOutputYellow tests control message outputs when a fixture
// fails to reset.
func TestInternalStackOutputYellow(t *gotesting.T) {
	ctx := context.Background()
	sink := outputtest.NewSink()
	stack := fixture.NewInternalStack(&fixture.Config{GracePeriod: planner.DefaultGracePeriod}, sink)

	newLoggingFixture := func(id int, reset bool) *testing.FixtureInstance {
		return &testing.FixtureInstance{
			Name: fmt.Sprintf("fixt%d", id),
			Impl: testfixture.New(
				testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					logging.Infof(ctx, "SetUp %d", id)
					return nil
				}),
				testfixture.WithReset(func(ctx context.Context) error {
					logging.Infof(ctx, "Reset %d", id)
					if !reset {
						return errors.New("failure")
					}
					return nil
				}),
				testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
					logging.Infof(ctx, "TearDown %d", id)
				})),
		}
	}
	fixt1 := newLoggingFixture(1, true)
	fixt2 := newLoggingFixture(2, false)
	fixt3 := newLoggingFixture(3, true)

	// Push and pop three fixture. Second fixture fails to reset.
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
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}
