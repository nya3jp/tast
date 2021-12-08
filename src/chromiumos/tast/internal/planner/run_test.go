// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/devserver/devservertest"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner/internal/output/outputtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/testing/testfixture"
	"chromiumos/tast/testutil"
)

// runTestsAndReadAll runs tests and returns a slice of control messages written to the output.
func runTestsAndReadAll(t *gotesting.T, tests []*testing.TestInstance, pcfg *Config) []protocol.Event {
	t.Helper()

	sink := outputtest.NewSink()
	if err := RunTestsLegacy(context.Background(), tests, sink, pcfg); err != nil {
		t.Logf("RunTests: %v", err) // improve debuggability
	}
	return sink.ReadAll()
}

// testPre implements Precondition for unit tests.
type testPre struct {
	prepareFunc func(context.Context, *testing.PreState) interface{}
	closeFunc   func(context.Context, *testing.PreState)
	name        string // name to return from String
}

func (p *testPre) Prepare(ctx context.Context, s *testing.PreState) interface{} {
	if p.prepareFunc != nil {
		return p.prepareFunc(ctx, s)
	}
	return nil
}

func (p *testPre) Close(ctx context.Context, s *testing.PreState) {
	if p.closeFunc != nil {
		p.closeFunc(ctx, s)
	}
}

func (p *testPre) Timeout() time.Duration { return time.Minute }

func (p *testPre) String() string { return p.name }

func TestRunSuccess(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	od := filepath.Join(td, "out")

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{Dirs: &protocol.RunDirectories{OutDir: od}})

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto(), OutDir: filepath.Join(od, "pkg.Test")},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}

	if fi, err := os.Stat(filepath.Join(od, tests[0].Name)); err != nil {
		t.Errorf("Out dir %v not accessible after testing: %v", od, err)
	} else if mode := fi.Mode()&os.ModePerm | os.ModeSticky; mode != 0777|os.ModeSticky {
		t.Errorf("Out dir %v has mode 0%o; want 0%o", od, mode, 0777|os.ModeSticky)
	}
}

func TestRunPanic(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Func:    func(context.Context, *testing.State) { panic("intentional panic") },
		Timeout: time.Minute,
	}}
	msgs := runTestsAndReadAll(t, tests, &Config{})
	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "Panic: intentional panic"}},
		&protocol.EntityEndEvent{EntityName: "pkg.Test"},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunDeadline(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			// Wait for the context to report that the deadline has been hit.
			<-ctx.Done()
			s.Error("Saw timeout within test")
		},
		Timeout: time.Millisecond,
	}}
	gracePeriod := 10 * time.Second
	msgs := runTestsAndReadAll(t, tests, &Config{CustomGracePeriod: &gracePeriod})
	// The error that was reported by the test after its deadline was hit
	// but within the exit delay should be available.
	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "Saw timeout within test"}},
		&protocol.EntityEndEvent{EntityName: "pkg.Test"},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunLogAfterTimeout(t *gotesting.T) {
	cont := make(chan bool)
	done := make(chan bool)
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			// Report when we're done, either after completing or after panicking before completion.
			completed := false
			defer func() { done <- completed }()

			// Ignore the deadline and wait until we're told to continue.
			<-ctx.Done()
			<-cont
			s.Log("Done waiting")
			completed = true
		},
		Timeout: time.Millisecond,
	}}

	gracePeriod := time.Millisecond
	msgs := runTestsAndReadAll(t, tests, &Config{CustomGracePeriod: &gracePeriod})

	// Tell the test to continue even though Run has already returned. The output stream should
	// still be writable so as to avoid a panic when the test writes to it (https://crbug.com/853406),
	// but they are dropped.
	cont <- true
	if completed := <-done; !completed {
		t.Error("Test function didn't complete")
	}

	// An error is written with a goroutine dump.
	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "Test did not return on timeout (see log for goroutine dump)"}},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "Dumping all goroutines"},
		// A goroutine dump follows. Do not compare them as the content is undeterministic.
	}
	if diff := cmp.Diff(msgs[:len(want)], want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunLateWriteFromGoroutine(t *gotesting.T) {
	// Run a test that calls s.Log from a goroutine after the test has finished.
	start := make(chan struct{}) // tells goroutine to start
	end := make(chan struct{})   // announces goroutine is done
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			go func() {
				<-start
				s.Log("This message should be still reported")
				close(end)
			}()
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	// Tell the goroutine to start and wait for it to finish.
	close(start)
	<-end

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		// Log message from the goroutine is not reported.
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunSoftwareDeps(t *gotesting.T) {
	const (
		validDep   = "valid"
		missingDep = "missing"
		unregDep   = "unreg"
	)

	nopFunc := func(context.Context, *testing.State) {}
	test1 := &testing.TestInstance{Name: "pkg.Test1", SoftwareDeps: []string{validDep}, Func: nopFunc, Timeout: time.Minute}
	test2 := &testing.TestInstance{Name: "pkg.Test2", SoftwareDeps: []string{missingDep}, Func: nopFunc, Timeout: time.Minute}
	test3 := &testing.TestInstance{Name: "pkg.Test3", SoftwareDeps: []string{unregDep}, Func: nopFunc, Timeout: time.Minute}
	tests := []*testing.TestInstance{test1, test2, test3}

	cfg := &Config{
		Features: &protocol.Features{
			CheckDeps: true,
			Dut: &protocol.DUTFeatures{
				Software: &protocol.SoftwareFeatures{
					Available:   []string{validDep},
					Unavailable: []string{missingDep},
				},
			},
		},
	}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: test2.EntityProto()},
		&protocol.EntityEndEvent{EntityName: test2.Name, Skip: &protocol.Skip{Reasons: []string{"missing SoftwareDeps: missing"}}},
		&protocol.EntityStartEvent{Entity: test3.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: test3.Name, Error: &protocol.Error{Reason: "unknown SoftwareDeps: unreg"}},
		&protocol.EntityEndEvent{EntityName: test3.Name},
		&protocol.EntityStartEvent{Entity: test1.EntityProto()},
		&protocol.EntityEndEvent{EntityName: test1.Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunVarDeps(t *gotesting.T) {
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	type verdict string
	const (
		pass verdict = "pass"
		fail verdict = "fail"
		skip verdict = "skip"
	)
	for _, tc := range []struct {
		name             string
		maybeMissingVars string
		givenVars        []string
		varDeps          []string
		want             verdict
	}{{
		name:      "simple pass",
		givenVars: []string{"foo"},
		varDeps:   []string{"foo"},
		want:      pass,
	}, {
		name:    "simple fail",
		varDeps: []string{"foo"},
		want:    fail,
	}, {
		name:             "simple skip",
		maybeMissingVars: "foo",
		varDeps:          []string{"foo"},
		want:             skip,
	}, {
		name:             "expected missing vars",
		maybeMissingVars: "foo",
		givenVars:        []string{"bar"},
		varDeps:          []string{"bar", "foo"},
		want:             skip,
	}, {
		name:             "unexpected missing vars",
		maybeMissingVars: "foo",
		varDeps:          []string{"bar", "foo"},
		want:             fail,
	}, {
		name:             "simple regex",
		maybeMissingVars: `foo\..*`,
		varDeps:          []string{"foo.a", "foo.b.c"},
		want:             skip,
	}, {
		name:             "complex regex",
		maybeMissingVars: `(bar|foo\..*)`,
		varDeps:          []string{"bar", "foo.a", "foo.b.c"},
		want:             skip,
	}, {
		name:             "no substring match",
		maybeMissingVars: `foo`,
		varDeps:          []string{"foobar"},
		want:             fail,
	}} {
		t.Run(tc.name, func(t *gotesting.T) {
			test := &testing.TestInstance{
				Name: "t.T",
				Func: func(ctx context.Context, s *testing.State) {
					defer func() {
						// s.RequiredVar() panics on failure.
						if r := recover(); r != nil {
							t.Errorf("panicked: %v", r)
						}
					}()
					for _, v := range tc.varDeps {
						if got, want := s.RequiredVar(v), "val"; got != want {
							t.Errorf("s.RequiredVar(%q)=%q, want %q", v, got, want)
						}
					}
				},
				VarDeps: tc.varDeps,
				Timeout: time.Minute,
			}

			vars := make(map[string]string)
			for _, s := range tc.givenVars {
				vars[s] = "val"
			}

			cfg := &Config{
				Features: &protocol.Features{
					CheckDeps: true,
					Infra: &protocol.InfraFeatures{
						Vars:             vars,
						MaybeMissingVars: tc.maybeMissingVars,
					},
				},
			}

			msgs := runTestsAndReadAll(t, []*testing.TestInstance{test}, cfg)

			if got := func() verdict {
				for _, msg := range msgs {
					switch m := msg.(type) {
					case *protocol.EntityEndEvent:
						if len(m.GetSkip().GetReasons()) > 0 {
							return skip
						}
						return pass
					case *protocol.EntityErrorEvent:
						t.Logf("Got error: %v", *m.GetError())
						return fail
					}
				}
				t.Fatal("Unexpected end of message")
				panic("BUG: unreachable")
			}(); got != tc.want {
				t.Errorf("Got verdict %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunSkipStages(t *gotesting.T) {
	// action specifies an action performed in a stage.
	type action int
	const (
		pass    action = iota
		doError        // call State.Error
		doFatal        // call State.Fatal
		doPanic        // call panic()
		noCall         // stage should be skipped
	)

	// testBehavior specifies the behavior of a test in each of its stages.
	type testBehavior struct {
		pre            *testPre
		preTestAction  action // Config.TestHook
		prepareAction  action // Precondition.Prepare
		testAction     action // Test.Func
		closeAction    action // Precondition.Close
		postTestAction action // Return of Config.TestHook
	}

	// pre1 and pre2 are preconditions used in tests. prepareFunc and closeFunc
	// are filled in each subtest.
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}

	evcmps := []cmp.Option{
		protocmp.Transform(),
		protocmp.IgnoreEmptyMessages(),
		protocmp.IgnoreFields(&protocol.Entity{}, "legacy_data"),
		protocmp.IgnoreFields(&protocol.EntityStartEvent{}, "out_dir"),
	}

	for _, tc := range []struct {
		name  string
		tests []testBehavior
		want  []protocol.Event
	}{
		{
			name: "no precondition",
			tests: []testBehavior{
				{nil, pass, noCall, pass, noCall, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "passes",
			tests: []testBehavior{
				{pre1, pass, pass, pass, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "pretest fails",
			tests: []testBehavior{
				{pre1, doError, noCall, noCall, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "preTest: Intentional error"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "pretest panics",
			tests: []testBehavior{
				{pre1, doPanic, noCall, noCall, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "Panic: preTest: Intentional panic"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "prepare fails",
			tests: []testBehavior{
				{pre1, pass, doError, noCall, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "[Precondition failure] prepare: Intentional error"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "prepare panics",
			tests: []testBehavior{
				{pre1, pass, doPanic, noCall, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "[Precondition failure] Panic: prepare: Intentional panic"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "same precondition",
			tests: []testBehavior{
				{pre1, pass, pass, pass, noCall, pass},
				{pre1, pass, pass, pass, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "1"}},
				&protocol.EntityLogEvent{EntityName: "1", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "1"},
			},
		},
		{
			name: "different preconditions",
			tests: []testBehavior{
				{pre1, pass, pass, pass, pass, pass},
				{pre2, pass, pass, pass, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "1"}},
				&protocol.EntityLogEvent{EntityName: "1", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Preparing precondition "pre2"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Closing precondition "pre2"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "1"},
			},
		},
		{
			name: "first prepare fails",
			tests: []testBehavior{
				{pre1, pass, doError, noCall, noCall, pass},
				{pre1, pass, pass, pass, pass, pass},
			},
			want: []protocol.Event{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "[Precondition failure] prepare: Intentional error"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "1"}},
				&protocol.EntityLogEvent{EntityName: "1", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "1"},
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			type state interface {
				Logf(fmt string, args ...interface{})
				Errorf(fmt string, args ...interface{})
				Fatalf(fmt string, args ...interface{})
			}
			doAction := func(s state, a action, stage string) {
				switch a {
				case pass:
					s.Logf("%s: OK", stage)
				case doError:
					s.Errorf("%s: Intentional error", stage)
				case doFatal:
					s.Fatalf("%s: Intentional fatal", stage)
				case doPanic:
					panic(fmt.Sprintf("%s: Intentional panic", stage))
				case noCall:
					t.Errorf("%s: Called unexpectedly", stage)
				}
			}

			currentBehavior := func(s interface{ OutDir() string }) testBehavior {
				// Abuse OutDir to tell which test we're running currently.
				i, err := strconv.Atoi(filepath.Base(s.OutDir()))
				if err != nil {
					t.Fatal("Failed to parse OutDir: ", err)
				}
				return tc.tests[i]
			}

			var tests []*testing.TestInstance
			for i, tb := range tc.tests {
				test := &testing.TestInstance{
					Name: strconv.Itoa(i),
					Func: func(ctx context.Context, s *testing.State) {
						doAction(s, currentBehavior(s).testAction, "test")
					},
					Timeout: time.Minute,
				}
				// We can't just do "test.Pre =tbc.pre" here. See e.g. https://tour.golang.org/methods/12:
				// "Note that an interface value that holds a nil concrete value is itself non-nil."
				if tb.pre != nil {
					test.Pre = tb.pre
				}
				tests = append(tests, test)
			}

			for _, pre := range []*testPre{pre1, pre2} {
				pre.prepareFunc = func(ctx context.Context, s *testing.PreState) interface{} {
					doAction(s, currentBehavior(s).prepareAction, "prepare")
					return nil
				}
				pre.closeFunc = func(ctx context.Context, s *testing.PreState) {
					doAction(s, currentBehavior(s).closeAction, "close")
				}
			}

			outDir := testutil.TempDir(t)
			defer os.RemoveAll(outDir)

			pcfg := &Config{
				Dirs: &protocol.RunDirectories{OutDir: outDir},
				TestHook: func(ctx context.Context, s *testing.TestHookState) func(context.Context, *testing.TestHookState) {
					doAction(s, currentBehavior(s).preTestAction, "preTest")
					return func(ctx context.Context, s *testing.TestHookState) {
						doAction(s, currentBehavior(s).postTestAction, "postTest")
					}
				},
			}
			msgs := runTestsAndReadAll(t, tests, pcfg)
			if diff := cmp.Diff(msgs, tc.want, evcmps...); diff != "" {
				t.Error("Output mismatch (-got +want):\n", diff)
			}
		})
	}
}

func buildLink(t *gotesting.T, url, data string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(data))
	ld := &extdata.LinkData{
		Type:      extdata.TypeStatic,
		StaticURL: url,
		Size:      int64(len(data)),
		SHA256Sum: hex.EncodeToString(hash[:]),
	}
	b, err := json.Marshal(ld)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunExternalData(t *gotesting.T) {
	const (
		file1URL  = "gs://bucket/file1.txt"
		file1Path = "pkg/data/file1.txt"
		file1Data = "data1"
		file2URL  = "gs://bucket/file2.txt"
		file2Path = "pkg/data/file2.txt"
		file2Data = "data2"
		file3URL  = "gs://bucket/file3.txt"
		file3Path = "pkg/data/file3.txt"
		file3Data = "data3"

		downloadFailURL  = "gs://bucket/fail.txt"
		downloadFailPath = "pkg/data/fail.txt"
	)

	for _, tc := range []struct {
		name         string
		mode         protocol.DownloadMode
		numDownloads int
	}{
		{"batch", protocol.DownloadMode_BATCH, 1},
		{"lazy", protocol.DownloadMode_LAZY, 4},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			ds, err := devservertest.NewServer(devservertest.Files([]*devservertest.File{
				{URL: file1URL, Data: []byte(file1Data)},
				{URL: file2URL, Data: []byte(file2Data)},
				{URL: file3URL, Data: []byte(file3Data)},
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer ds.Close()

			tmpDir := testutil.TempDir(t)
			defer os.RemoveAll(tmpDir)

			dataDir := filepath.Join(tmpDir, "data")
			if err := testutil.WriteFiles(dataDir, map[string]string{
				file1Path + testing.ExternalLinkSuffix:        buildLink(t, file1URL, file1Data),
				file2Path + testing.ExternalLinkSuffix:        buildLink(t, file2URL, file2Data),
				file3Path + testing.ExternalLinkSuffix:        buildLink(t, file3URL, file3Data),
				downloadFailPath + testing.ExternalLinkSuffix: buildLink(t, downloadFailURL, ""),
			}); err != nil {
				t.Fatal("WriteFiles: ", err)
			}

			numDownloads := 0

			fixt := &testing.FixtureInstance{
				Name: "fixt",
				Pkg:  "pkg",
				Impl: testfixture.New(),
				Data: []string{"file3.txt"},
			}
			fixt2 := &testing.FixtureInstance{
				Name: "fixt2",
				Pkg:  "pkg",
				Impl: testfixture.New(),
				Data: []string{"fail.txt"},
			}

			pcfg := &Config{
				Dirs: &protocol.RunDirectories{DataDir: dataDir},
				Features: &protocol.Features{
					CheckDeps: true,
					Dut: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Available:   []string{"dep2"},
							Unavailable: []string{"dep1"},
						},
					},
				},
				Service: &protocol.ServiceConfig{
					Devservers: []string{ds.URL},
				},
				DataFile: &protocol.DataFileConfig{
					DownloadMode: tc.mode,
				},
				BeforeDownload: func(ctx context.Context) {
					numDownloads++
				},
				Fixtures: map[string]*testing.FixtureInstance{
					"fixt":  fixt,
					"fixt2": fixt2,
				},
			}

			tests := []*testing.TestInstance{
				{
					Name:         "example.Test1",
					Pkg:          "pkg",
					Func:         func(ctx context.Context, s *testing.State) {},
					Fixture:      "fixt",
					SoftwareDeps: []string{"dep1"},
					Timeout:      time.Minute,
				},
				{
					Name: "example.Test2",
					Pkg:  "pkg",
					Func: func(ctx context.Context, s *testing.State) {
						fp := filepath.Join(dataDir, downloadFailPath+testing.ExternalErrorSuffix)
						_, err := os.Stat(fp)
						switch tc.mode {
						case protocol.DownloadMode_BATCH:
							// In DownloadBatch mode, external data files for Test3 are already downloaded.
							if err != nil {
								t.Errorf("In Test2: %v; want present", err)
							}
						case protocol.DownloadMode_LAZY:
							// In DownloadLazy mode, external data files for Test3 are not downloaded yet.
							if err == nil {
								t.Errorf("In Test2: %s exists; want missing", fp)
							} else if !os.IsNotExist(err) {
								t.Errorf("In Test2: %v; want missing", err)
							}
						}
					},
					Fixture:      "fixt",
					Data:         []string{"file2.txt"},
					SoftwareDeps: []string{"dep2"},
					Timeout:      time.Minute,
				},
				{
					Name:    "example.Test3",
					Pkg:     "pkg",
					Func:    func(ctx context.Context, s *testing.State) {},
					Fixture: "fixt",
					Data:    []string{"fail.txt"},
					Timeout: time.Minute,
				},
				{
					Name:    "example.Test4",
					Pkg:     "pkg",
					Func:    func(ctx context.Context, s *testing.State) {},
					Fixture: "fixt2",
					Timeout: time.Minute,
				},
			}

			msgs := runTestsAndReadAll(t, tests, pcfg)

			want := []protocol.Event{
				&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
				&protocol.EntityEndEvent{EntityName: tests[0].Name, Skip: &protocol.Skip{Reasons: []string{"missing SoftwareDeps: dep1"}}},
				&protocol.EntityStartEvent{Entity: fixt.EntityProto()},
				&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
				&protocol.EntityEndEvent{EntityName: tests[1].Name},
				&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
				&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "Required data file fail.txt missing: failed to download gs://bucket/fail.txt: file does not exist"}},
				&protocol.EntityEndEvent{EntityName: tests[2].Name},
				&protocol.EntityEndEvent{EntityName: fixt.Name},

				&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
				&protocol.EntityErrorEvent{EntityName: fixt2.Name, Error: &protocol.Error{Reason: "Required data file fail.txt missing: failed to download gs://bucket/fail.txt: file does not exist"}},
				&protocol.EntityEndEvent{EntityName: fixt2.Name},
				&protocol.EntityStartEvent{Entity: tests[3].EntityProto()},
				&protocol.EntityErrorEvent{EntityName: tests[3].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt2: Required data file fail.txt missing: failed to download gs://bucket/fail.txt: file does not exist"}},
				&protocol.EntityEndEvent{EntityName: tests[3].Name},
			}
			if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
				t.Error("Output mismatch (-got +want):\n", diff)
			}

			files, err := testutil.ReadFiles(dataDir)
			if err != nil {
				t.Fatal("ReadFiles: ", err)
			}
			wantFiles := map[string]string{
				// file1.txt is not downloaded since pkg.Test1 is not run.
				file1Path + testing.ExternalLinkSuffix:         buildLink(t, file1URL, file1Data),
				file2Path:                                      file2Data,
				file2Path + testing.ExternalLinkSuffix:         buildLink(t, file2URL, file2Data),
				downloadFailPath + testing.ExternalLinkSuffix:  buildLink(t, downloadFailURL, ""),
				downloadFailPath + testing.ExternalErrorSuffix: "failed to download gs://bucket/fail.txt: file does not exist",
				file3Path:                              file3Data,
				file3Path + testing.ExternalLinkSuffix: buildLink(t, file3URL, file3Data),
			}
			if diff := cmp.Diff(files, wantFiles); diff != "" {
				t.Error("Data directory mismatch (-got +want):\n", diff)
			}

			if numDownloads != tc.numDownloads {
				t.Errorf("Unexpected number of download attempts: got %d, want %d", numDownloads, tc.numDownloads)
			}
		})
	}
}

func TestLazyDownloadPurgeable(t *gotesting.T) {
	const (
		file1URL  = "gs://bucket/file1.txt"
		file1Path = "pkg/data/file1.txt"
		file2URL  = "gs://bucket/file2.txt"
		file2Path = "pkg/data/file2.txt"
		file3URL  = "gs://bucket/file3.txt"
		file3Path = "pkg/data/file3.txt"
	)

	ds, err := devservertest.NewServer(devservertest.Files([]*devservertest.File{
		{URL: file1URL},
		{URL: file2URL},
		{URL: file3URL},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	dataDir := filepath.Join(tmpDir, "data")
	if err := testutil.WriteFiles(dataDir, map[string]string{
		file1Path + testing.ExternalLinkSuffix: buildLink(t, file1URL, ""),
		file2Path + testing.ExternalLinkSuffix: buildLink(t, file2URL, ""),
		file3Path + testing.ExternalLinkSuffix: buildLink(t, file3URL, ""),
		file3Path:                              "", // file3 already exists
	}); err != nil {
		t.Fatal("WriteFiles: ", err)
	}

	ti := func(name string, data []string, fixture string) *testing.TestInstance {
		return &testing.TestInstance{
			Name:    name,
			Fixture: fixture,
			Data:    data,
			Pkg:     "pkg",
			Func:    func(ctx context.Context, s *testing.State) {},
			Timeout: time.Minute,
		}
	}
	tests := []*testing.TestInstance{
		ti("example.Tetst1", []string{"file1.txt"}, ""),
		ti("example.Tetst2", []string{"file2.txt"}, ""),
		ti("example.Tetst3", []string{"file2.txt"}, "fixt"),
		ti("example.Tetst4", []string{}, "fixt"),
		ti("example.Tetst5", []string{}, "fixt2"),
	}

	var got [][]string

	pcfg := &Config{
		Dirs:     &protocol.RunDirectories{DataDir: dataDir},
		Service:  &protocol.ServiceConfig{Devservers: []string{ds.URL}},
		DataFile: &protocol.DataFileConfig{DownloadMode: protocol.DownloadMode_LAZY},
		TestHook: func(ctx context.Context, s *testing.TestHookState) func(context.Context, *testing.TestHookState) {
			got = append(got, s.Purgeable())
			return nil
		},
		Fixtures: map[string]*testing.FixtureInstance{
			"fixt": {
				Pkg:  "pkg",
				Name: "fixt",
				Impl: testfixture.New(),
				Data: []string{"file3.txt"},
			},
			"fixt2": {
				Pkg:  "pkg",
				Name: "fixt2",
				Impl: testfixture.New(),
				Data: []string{},
			},
		},
	}

	runTestsAndReadAll(t, tests, pcfg)

	abs := func(p string) string { return filepath.Join(dataDir, p) }
	want := [][]string{
		{abs(file3Path)},
		{abs(file1Path), abs(file3Path)},
		{abs(file1Path)},
		{abs(file1Path), abs(file2Path)}, // file3Path is used by fixt
		{abs(file1Path), abs(file2Path), abs(file3Path)},
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Purgeable mismatch: (-got +want):\n", diff)
	}
}

func TestRunCloudStorage(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			if s.CloudStorage() == nil {
				t.Error("testing.State.CloudStorage is nil")
			}
		},
	}}
	cfg := &Config{}
	_ = runTestsAndReadAll(t, tests, cfg)
}

func TestRunFixture(t *gotesting.T) {
	const (
		val1 = "val1"
		val2 = "val2"
	)
	fixt1 := &testing.FixtureInstance{
		Name: "fixt1",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				logging.Info(ctx, "fixt1 SetUp")
				return val1
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "fixt1 Reset")
				return nil
			}),
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt1 PreTest")
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt1 PostTest")
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				logging.Info(ctx, "fixt1 TearDown")
			})),
	}
	fixt2 := &testing.FixtureInstance{
		Name: "fixt2",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				logging.Info(ctx, "fixt2 SetUp")
				return val2
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "fixt2 Reset")
				return nil
			}),
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt2 PreTest")
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt2 PostTest")
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				logging.Info(ctx, "fixt2 TearDown")
			})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name: "pkg.Test0",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 0")
			if val := s.FixtValue(); val != nil {
				t.Errorf("pkg.Test0: FixtValue() = %v; want nil", val)
			}
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 1")
			if val := s.FixtValue(); val != val1 {
				t.Errorf("pkg.Test1: FixtValue() = %v; want %v", val, val1)
			}
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 2")
			if val := s.FixtValue(); val != val2 {
				t.Errorf("pkg.Test2: FixtValue() = %v; want %v", val, val2)
			}
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		// pkg.Test0 simply runs.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Test 0"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		// pkg.Test1 depends on fixt1.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "fixt1 SetUp"},
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "fixt1 PreTest"},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "Test 1"},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "fixt1 PostTest"},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		// fixt1 is reset.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "fixt1 Reset"},
		// pkg.Test2 depends on fixt2, which in turn depends on fixt1.
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "fixt2 SetUp"},
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt1 PreTest"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt2 PreTest"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "Test 2"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt2 PostTest"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt1 PostTest"},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// fixt1 and fixt2 are torn down.
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "fixt2 TearDown"},
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "fixt1 TearDown"},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureSetUpFailure(t *gotesting.T) {
	fixt1 := &testing.FixtureInstance{
		Name: "fixt1",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Setup failure 1")
				s.Error("Setup failure 2")
				return nil
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "fixt1 Reset")
				return nil
			}),
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt1 PreTest")
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt1 PostTest")
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				logging.Info(ctx, "fixt1 TearDown")
			})),
	}
	fixt2 := &testing.FixtureInstance{
		Name: "fixt2",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				logging.Info(ctx, "fixt2 SetUp")
				return nil
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "fixt2 Reset")
				return nil
			}),
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt2 PreTest")
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt2 PostTest")
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				logging.Info(ctx, "fixt2 TearDown")
			})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name: "pkg.Test0",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 0")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 1")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 2")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		// pkg.Test0 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Test 0"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		// fixt1 fails to set up.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: fixt1.Name, Error: &protocol.Error{Reason: "Setup failure 1"}},
		&protocol.EntityErrorEvent{EntityName: fixt1.Name, Error: &protocol.Error{Reason: "Setup failure 2"}},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
		// All tests depending on fixt1 fail.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[1].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 1"}},
		&protocol.EntityErrorEvent{EntityName: tests[1].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 2"}},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 1"}},
		&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 2"}},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixturePreTestFailure(t *gotesting.T) {
	fixt1 := &testing.FixtureInstance{
		Name: "fixt1",
		Impl: testfixture.New(
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt1 PreTest")
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt1 PostTest")
			})),
	}
	fixt2 := &testing.FixtureInstance{
		Name: "fixt2",
		Impl: testfixture.New(
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				s.Error("fixt2 PreTest fail")
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				logging.Info(ctx, "fixt2 PostTest")
			})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "fixt1 PreTest"},
		&protocol.EntityErrorEvent{EntityName: tests[0].Name, Error: &protocol.Error{Reason: "fixt2 PreTest fail"}},
		// fixt2 PostTest and test should not run.
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "fixt1 PostTest"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureRemoteSetUpFailure(t *gotesting.T) {
	cfg := &Config{
		StartFixtureName: "remoteFixt",
		StartFixtureImpl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Remote failure")
				return nil
			})),
	}
	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "remoteFixt",
		Func: func(ctx context.Context, s *testing.State) {
			t.Errorf("pkg.Test run unexpectedly")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{Error: &protocol.Error{Reason: "[Fixture failure] remoteFixt: Remote failure"}, EntityName: "pkg.Test"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureResetFailure(t *gotesting.T) {
	fixt1 := &testing.FixtureInstance{
		Name: "fixt1",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				logging.Info(ctx, "SetUp 1")
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				logging.Info(ctx, "TearDown 1")
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "Reset 1")
				return errors.New("failure") // always fail
			}),
		),
	}
	fixt2 := &testing.FixtureInstance{
		Name: "fixt2",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				logging.Info(ctx, "SetUp 2")
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				logging.Info(ctx, "TearDown 2")
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "Reset 2")
				return nil // always succeed
			}),
		),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name: "pkg.Test0",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 0")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 1")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 2")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test3",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 3")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		// pkg.Test0 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Test 0"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		// fixt1 sets up successfully.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "SetUp 1"},
		// pkg.Test1 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "Test 1"},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		// fixt1 fails to reset. It is restarted.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Fixture failed to reset: failure; recovering"},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "TearDown 1"},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
		// fixt1 and fixt2 set up successfully.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "SetUp 1"},
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "SetUp 2"},
		// pkg.Test2 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "Test 2"},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// fixt1 fails to reset. fixt1 and fixt2 are restarted.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Fixture failed to reset: failure; recovering"},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "TearDown 2"},
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "TearDown 1"},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
		// fixt1 and fixt2 set up successfully.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "SetUp 1"},
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "SetUp 2"},
		// pkg.Test3 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[3].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[3].Name, Text: "Test 3"},
		&protocol.EntityEndEvent{EntityName: tests[3].Name},
		// Fixtures are torn down.
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "TearDown 2"},
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "TearDown 1"},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureMinimumReset(t *gotesting.T) {
	fixt1 := &testing.FixtureInstance{
		Name: "fixt1",
		Impl: testfixture.New(testfixture.WithReset(func(ctx context.Context) error {
			logging.Info(ctx, "Reset 1")
			return nil
		})),
	}
	fixt2 := &testing.FixtureInstance{
		Name: "fixt2",
		Impl: testfixture.New(testfixture.WithReset(func(ctx context.Context) error {
			logging.Info(ctx, "Reset 2")
			return nil
		})),
		Parent: "fixt1",
	}
	fixt3 := &testing.FixtureInstance{
		Name: "fixt3",
		Impl: testfixture.New(testfixture.WithReset(func(ctx context.Context) error {
			logging.Info(ctx, "Reset 3")
			return nil
		})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
			fixt3.Name: fixt3,
		},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test0",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test3",
		Fixture: "fixt2",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test4",
		Fixture: "fixt2",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test5",
		Fixture: "fixt3",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test6",
		Fixture: "fixt3",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		// pkg.Test0 runs.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		// fixt1 starts.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		// pkg.Test1 runs.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		// fixt1 is reset for pkg.Test2.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		// pkg.Test2 runs.
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// fixt1 is reset for pkg.Test3. fixt2 is NOT reset.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		// fixt2 starts.
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		// pkg.Test3 runs.
		&protocol.EntityStartEvent{Entity: tests[3].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[3].Name},
		// fixt1 and fixt2 are reset for pkg.Test4.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "Reset 2"},
		// pkg.Test4 runs.
		&protocol.EntityStartEvent{Entity: tests[4].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[4].Name},
		// fixt2 ends.
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		// fixt1 is reset for pkg.Test5. fixt2 is NOT reset.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		// fixt3 starts.
		&protocol.EntityStartEvent{Entity: fixt3.EntityProto()},
		// pkg.Test5 runs.
		&protocol.EntityStartEvent{Entity: tests[5].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[5].Name},
		// fixt1 and fixt3 are reset for pkg.Test5.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt3.Name, Text: "Reset 3"},
		// pkg.Test6 runs.
		&protocol.EntityStartEvent{Entity: tests[6].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[6].Name},
		// fixt3 and fixt1 end. They are NOT reset.
		&protocol.EntityEndEvent{EntityName: fixt3.Name},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureMissing(t *gotesting.T) {
	fixt1 := &testing.FixtureInstance{
		Name:   "fixt1",
		Impl:   testfixture.New(),
		Parent: "fixt0", // no such fixture
	}
	fixt2 := &testing.FixtureInstance{
		Name:   "fixt2",
		Impl:   testfixture.New(),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test0",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []protocol.Event{
		// Orphan tests are reported first, sorted by name.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[1].Name, Error: &protocol.Error{Reason: "Fixture \"fixt0\" not found"}},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "Fixture \"fixt0\" not found"}},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// Valid tests are run.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureVars(t *gotesting.T) {
	const (
		declaredVarName   = "declared"
		declaredVarValue  = "foo"
		undeclaredVarName = "undeclared"
	)

	fixt := &testing.FixtureInstance{
		Name: "fixt",
		Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			func() {
				defer func() {
					if recover() != nil {
						t.Errorf("Failed to access variable %q", declaredVarName)
					}
				}()
				if value, ok := s.Var(declaredVarName); !ok {
					t.Errorf("Variable %q not found", declaredVarName)
				} else if value != declaredVarValue {
					t.Errorf("Variable %q = %q; want %q", declaredVarName, value, declaredVarValue)
				}
			}()
			func() {
				// Brace for panic.
				defer func() {
					recover()
				}()
				s.Var(undeclaredVarName)
				t.Errorf("Variable %q could be accessed unexpectedly", undeclaredVarName)
			}()
			return nil
		})),
		Vars: []string{declaredVarName},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "fixt",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{fixt.Name: fixt},
		Features: &protocol.Features{
			Infra: &protocol.InfraFeatures{
				Vars: map[string]string{
					declaredVarName:   declaredVarValue,
					undeclaredVarName: "forbidden",
				},
			},
		},
	}

	runTestsAndReadAll(t, tests, cfg)
}

func TestRunFixtureTestContext(t *gotesting.T) {
	var ctxPreTest context.Context

	var wg sync.WaitGroup
	wg.Add(2)

	fixt := &testing.FixtureInstance{
		Name: "fixt",
		Impl: testfixture.New(testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
			if err := s.TestContext().Err(); err != nil {
				t.Errorf("s.TestContext() is canceled: %v", err)
			}
			ctxPreTest = s.TestContext()
			go func() {
				<-s.TestContext().Done()
				wg.Done()
			}()
		}), testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
			if err := ctxPreTest.Err(); err != nil {
				t.Errorf("Test context in PreTest is canceled: %v", err)
			}
			if err := s.TestContext().Err(); err != nil {
				t.Errorf("s.TestContext() is canceled: %v", err)
			}
			go func() {
				<-s.TestContext().Done()
				wg.Done()
			}()
		}), testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
			// TestContext must be cancelled as soon as PostTest finishes.
			wg.Wait()
		})),
	}
	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "fixt",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}}

	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{fixt.Name: fixt},
	}
	runTestsAndReadAll(t, tests, cfg)
	wg.Wait()
}

func TestRunFixtureData(t *gotesting.T) {
	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)

	const fileName = "file.txt"
	testutil.WriteFiles(dataDir, map[string]string{
		filepath.Join("pkg/data", fileName): "42",
	})

	fixt := &testing.FixtureInstance{
		Name: "fixt",
		Pkg:  "pkg",
		Data: []string{fileName},
		Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			if got, want := s.DataPath(fileName), filepath.Join(dataDir, "pkg/data", fileName); got != want {
				t.Errorf("s.DataPath(%q) = %v, want %v", fileName, got, want)
			}
			func() {
				// Brace for panic.
				defer func() {
					recover()
				}()
				s.DataPath("unknown.txt")
				t.Errorf(`Data "unknown.txt" could be accessed unexpectedly`)
			}()

			srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/" + fileName)
			if err != nil {
				t.Error(err)
			}
			defer resp.Body.Close()
			if got, err := ioutil.ReadAll(resp.Body); err != nil {
				t.Error(err)
			} else if string(got) != "42" {
				t.Errorf(`Got %v, want "42"`, got)
			}
			return nil
		}), testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
			if got, want := s.DataPath(fileName), filepath.Join(dataDir, "pkg/data", fileName); got != want {
				t.Errorf("s.DataPath(%q) = %v, want %v", fileName, got, want)
			}
		})),
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "fixt",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	cfg := &Config{
		Fixtures: map[string]*testing.FixtureInstance{fixt.Name: fixt},
		Dirs:     &protocol.RunDirectories{DataDir: dataDir},
	}

	runTestsAndReadAll(t, tests, cfg)
}

func TestRunPrecondition(t *gotesting.T) {
	type data struct{}
	preData := &data{}

	// The test should be able to access the data via State.PreValue.
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		// Use a precondition that returns the struct that we declared earlier from its Prepare method.
		Pre: &testPre{
			name:        "pre",
			prepareFunc: func(context.Context, *testing.PreState) interface{} { return preData },
		},
		Func: func(ctx context.Context, s *testing.State) {
			if s.PreValue() == nil {
				s.Fatal("Precondition value not supplied")
			} else if d, ok := s.PreValue().(*data); !ok {
				s.Fatal("Precondition value didn't have expected type")
			} else if d != preData {
				s.Fatalf("Got precondition value %v; want %v", d, preData)
			}
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: `Preparing precondition "pre"`},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: `Closing precondition "pre"`},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunPreconditionWithSkips(t *gotesting.T) {
	const dep = "dep"

	var pre1Closed, pre2Closed bool
	pre1 := &testPre{
		name:      "pre1",
		closeFunc: func(context.Context, *testing.PreState) { pre1Closed = true },
	}
	pre2 := &testPre{
		name:      "pre2",
		closeFunc: func(context.Context, *testing.PreState) { pre2Closed = true },
	}

	// Make the last test using each precondition get skipped due to
	// missing software dependencies.
	nopFunc := func(context.Context, *testing.State) {}
	tests := []*testing.TestInstance{
		{Name: "pkg.Test1", Func: nopFunc, Pre: pre1},
		{Name: "pkg.Test2", Func: nopFunc, Pre: pre1},
		{Name: "pkg.Test3", Func: nopFunc, Pre: pre1, SoftwareDeps: []string{dep}},
		{Name: "pkg.Test4", Func: nopFunc, Pre: pre2},
		{Name: "pkg.Test5", Func: nopFunc, Pre: pre2},
		{Name: "pkg.Test6", Func: nopFunc, Pre: pre2, SoftwareDeps: []string{dep}},
	}

	cfg := &Config{
		Features: &protocol.Features{
			CheckDeps: true,
			Dut: &protocol.DUTFeatures{
				Software: &protocol.SoftwareFeatures{
					Unavailable: []string{dep},
				},
			},
		},
	}

	_ = runTestsAndReadAll(t, tests, cfg)
	if !pre1Closed {
		t.Error("pre1 was not closed")
	}
	if !pre2Closed {
		t.Error("pre2 was not closed")
	}
}

func TestRunPreconditionContext(t *gotesting.T) {
	var prevCtx context.Context

	prepareFunc := func(ctx context.Context, s *testing.PreState) interface{} {
		pctx := s.PreCtx()
		if prevCtx == nil {
			// This is the first test. Save pctx.
			prevCtx = pctx
		} else {
			// This is the second test. Ensure prevCtx is still alive.
			if err := prevCtx.Err(); err != nil {
				t.Error("Prepare (second test): PreCtx was canceled: ", err)
			}
		}

		logging.Info(pctx, "Log via PreCtx")

		if _, ok := testcontext.SoftwareDeps(pctx); !ok {
			t.Error("ContextSoftwareDeps unavailable")
		}
		if _, ok := testcontext.OutDir(pctx); ok {
			t.Error("ContextOutDir available")
		}
		return nil
	}

	closeFunc := func(ctx context.Context, s *testing.PreState) {
		if prevCtx != nil {
			if err := prevCtx.Err(); err != nil {
				t.Error("Close: PreCtx was canceled: ", err)
			}
		}
	}

	testFunc := func(ctx context.Context, s *testing.State) {}

	pre := &testPre{
		name:        "pre",
		prepareFunc: prepareFunc,
		closeFunc:   closeFunc,
	}

	tests := []*testing.TestInstance{
		{Name: "t1", Pre: pre, Timeout: time.Minute, Func: testFunc},
		{Name: "t2", Pre: pre, Timeout: time.Minute, Func: testFunc},
	}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: `Preparing precondition "pre"`},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Log via PreCtx"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: `Preparing precondition "pre"`},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "Log via PreCtx"},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: `Closing precondition "pre"`},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
	if prevCtx.Err() == nil {
		t.Error("Context not cancelled")
	}
}

type runPhase int

const (
	phasePreTestFunc runPhase = iota
	phasePrepareFunc
	phaseTestFunc
	phaseSubtestFunc
	phaseCloseFunc
	phasePostTestFunc
)

func (p runPhase) String() string {
	switch p {
	case phasePreTestFunc:
		return "preTestFunc"
	case phasePrepareFunc:
		return "prepareFunc"
	case phaseTestFunc:
		return "testFunc"
	case phaseSubtestFunc:
		return "subtestFunc"
	case phaseCloseFunc:
		return "closeFunc"
	case phasePostTestFunc:
		return "postTestFunc"
	default:
		return "unknown"
	}
}

func TestRunHasError(t *gotesting.T) {
	type stateLike interface {
		Error(args ...interface{})
		HasError() bool
	}

	for _, failIn := range []runPhase{
		phasePreTestFunc,
		phasePrepareFunc,
		phaseTestFunc,
		phaseSubtestFunc,
		phaseCloseFunc,
		phasePostTestFunc,
	} {
		t.Run(fmt.Sprintf("Fail in %s", failIn), func(t *gotesting.T) {
			onPhase := func(s stateLike, current runPhase) {
				got := s.HasError()
				want := current > failIn
				if got != want {
					t.Errorf("Phase %v: HasError()=%t; want %t", current, got, want)
				}
				if current == failIn {
					s.Error("Failure")
				}
			}

			pre := &testPre{
				prepareFunc: func(ctx context.Context, s *testing.PreState) interface{} {
					onPhase(s, phasePrepareFunc)
					return nil
				},
				closeFunc: func(ctx context.Context, s *testing.PreState) {
					onPhase(s, phaseCloseFunc)
				},
			}
			pcfg := &Config{
				TestHook: func(ctx context.Context, s *testing.TestHookState) func(context.Context, *testing.TestHookState) {
					onPhase(s, phasePreTestFunc)
					return func(ctx context.Context, s *testing.TestHookState) {
						onPhase(s, phasePostTestFunc)
					}
				},
			}
			testFunc := func(ctx context.Context, s *testing.State) {
				onPhase(s, phaseTestFunc)
				s.Run(ctx, "subtest", func(ctx context.Context, s *testing.State) {
					// Subtests are somewhat special; they do not inherit the error status from the parent state.
					if s.HasError() {
						t.Errorf("Phase %v: HasError()=true; want false", phaseSubtestFunc)
					}
					if failIn == phaseSubtestFunc {
						s.Error("Failure")
					}
				})
			}
			tests := []*testing.TestInstance{{Name: "t", Pre: pre, Timeout: time.Minute, Func: testFunc}}

			runTestsAndReadAll(t, tests, pcfg)
		})
	}
}

func TestAttachStateToContext(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			logging.Info(ctx, "msg ", 1)
			logging.Infof(ctx, "msg %d", 2)
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "msg 1"},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "msg 2"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunPlan(t *gotesting.T) {
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}
	fixt1 := &testing.FixtureInstance{Name: "fixt1", Impl: testfixture.New()}
	fixt2 := &testing.FixtureInstance{Name: "fixt2", Impl: testfixture.New(), Parent: "fixt1"}
	cfg := &Config{
		Features: &protocol.Features{
			CheckDeps: true,
			Dut: &protocol.DUTFeatures{
				Software: &protocol.SoftwareFeatures{
					Available:   []string{"yes"},
					Unavailable: []string{"no"},
				},
			},
		},
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	for _, tc := range []struct {
		name      string
		tests     []*testing.TestInstance
		wantOrder []string
	}{
		{
			name: "pre",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test6", Pre: pre2},
				{Name: "pkg.Test5", Pre: pre1},
				{Name: "pkg.Test4"},
				{Name: "pkg.Test3"},
				{Name: "pkg.Test2", Pre: pre1},
				{Name: "pkg.Test1", Pre: pre2},
			},
			wantOrder: []string{
				// Sorted by (precondition name, test name).
				"pkg.Test3",
				"pkg.Test4",
				"pkg.Test2",
				"pkg.Test5",
				"pkg.Test1",
				"pkg.Test6",
			},
		},
		{
			name: "fixt",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test6", Fixture: "fixt2"},
				{Name: "pkg.Test5", Fixture: "fixt1"},
				{Name: "pkg.Test4"},
				{Name: "pkg.Test3"},
				{Name: "pkg.Test2", Fixture: "fixt1"},
				{Name: "pkg.Test1", Fixture: "fixt2"},
			},
			wantOrder: []string{
				"pkg.Test3",
				"pkg.Test4",
				"fixt1",
				"pkg.Test2",
				"pkg.Test5",
				"fixt2",
				"pkg.Test1",
				"pkg.Test6",
			},
		},
		{
			name: "fixt_and_pre",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test1", Pre: pre1},
				{Name: "pkg.Test2", Fixture: "fixt1"},
				{Name: "pkg.Test3"},
			},
			wantOrder: []string{
				"pkg.Test3",
				"fixt1",
				"pkg.Test2",
				"pkg.Test1",
			},
		},
		{
			name: "deps",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test4", SoftwareDeps: []string{"yes"}},
				{Name: "pkg.Test3", SoftwareDeps: []string{"no"}},
				{Name: "pkg.Test2", SoftwareDeps: []string{"no"}, Pre: pre1},
				{Name: "pkg.Test1", SoftwareDeps: []string{"no"}, Pre: pre2},
			},
			wantOrder: []string{
				// Skipped tests come first.
				"pkg.Test1",
				"pkg.Test2",
				"pkg.Test3",
				"pkg.Test4",
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			msgs := runTestsAndReadAll(t, tc.tests, cfg)
			var order []string
			for _, msg := range msgs {
				if msg, ok := msg.(*protocol.EntityStartEvent); ok {
					order = append(order, msg.GetEntity().GetName())
				}
			}
			if diff := cmp.Diff(order, tc.wantOrder); diff != "" {
				t.Error("Test order mismatch (-got +want):\n", diff)
			}
		})
	}
}

type fixtureSteps string

const (
	fixtureSetup    fixtureSteps = "setup"
	fixtureReset    fixtureSteps = "reset"
	fixturePreTest  fixtureSteps = "preTest"
	fixturePostTest fixtureSteps = "postTest"
	fixtureTearDown fixtureSteps = "tearDown"
)

var allFixtureSteps = []fixtureSteps{
	fixtureSetup, fixtureReset, fixturePreTest, fixturePostTest, fixtureTearDown,
}

// recordingFixture is a FixtureImpl that records the labels passed to each function.
type recordingFixture struct {
	called        map[fixtureSteps]bool
	msgs          map[fixtureSteps]string
	expectedLabel string
}

func newRecordingFixture(label string) recordingFixture {
	return recordingFixture{
		called:        make(map[fixtureSteps]bool),
		msgs:          make(map[fixtureSteps]string),
		expectedLabel: label,
	}
}

func (f *recordingFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Recover from panic to continue testing other entities and lifecycle methods.
	defer func() {
		if e := recover(); e != nil {
			f.msgs[fixtureSetup] = e.(string)
		}
	}()
	f.called[fixtureSetup] = true
	testcontext.EnsureLabel(ctx, f.expectedLabel)
	return nil
}

func (f *recordingFixture) Reset(ctx context.Context) error {
	defer func() {
		if e := recover(); e != nil {
			f.msgs[fixtureReset] = e.(string)
		}
	}()
	f.called[fixtureReset] = true
	testcontext.EnsureLabel(ctx, f.expectedLabel)
	return nil
}

func (f *recordingFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	defer func() {
		if e := recover(); e != nil {
			f.msgs[fixturePreTest] = e.(string)
		}
	}()
	f.called[fixturePreTest] = true
	testcontext.EnsureLabel(ctx, f.expectedLabel)
}

func (f *recordingFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	defer func() {
		if e := recover(); e != nil {
			f.msgs[fixturePostTest] = e.(string)
		}
	}()
	f.called[fixturePostTest] = true
	testcontext.EnsureLabel(ctx, f.expectedLabel)
}

func (f *recordingFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	defer func() {
		if e := recover(); e != nil {
			f.msgs[fixtureSetup] = e.(string)
		}
	}()
	f.called[fixtureTearDown] = true
	testcontext.EnsureLabel(ctx, f.expectedLabel)
}

// TestRunEnsureLabel verifies that Labels can be read in tests and fixtures via the context variable.
func TestRunEnsureLabel(t *gotesting.T) {
	fixtureLabel1 := "label in fixture 1"
	fixtureLabel2 := "label in fixture 2"
	fixtureLabel3 := "label in fixture 3"
	testLabel := "label_in_test"
	impl1 := newRecordingFixture(fixtureLabel1)
	fixt1 := &testing.FixtureInstance{Name: "fixt1", Impl: &impl1, Labels: []string{fixtureLabel1}}
	impl2 := newRecordingFixture(fixtureLabel2)
	fixt2 := &testing.FixtureInstance{Name: "fixt2", Impl: &impl2, Labels: []string{fixtureLabel2}, Parent: "fixt1"}
	impl3 := newRecordingFixture(fixtureLabel3)
	fixt3 := &testing.FixtureInstance{Name: "fixt3", Impl: &impl3, Labels: []string{fixtureLabel3}, Parent: "fixt2"}
	cfg := &Config{
		Features: &protocol.Features{},
		Fixtures: map[string]*testing.FixtureInstance{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
			fixt3.Name: fixt3,
		},
	}

	// run 2 tests for the same fixture to run Reset() of the fixtures
	tests := []*testing.TestInstance{{
		Name:    "pkg.TestWithFixture1",
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute,
		Fixture: "fixt3",
		Labels:  []string{"irrelevant_label"},
	}, {
		Name: "pkg.TestWithFixture2",
		Func: func(ctx context.Context, _ *testing.State) {
			defer func() {
				if err := recover(); err != nil {
					t.Error("Failed to ensure label in TestWithFixture2:", err)
				}
			}()
			testcontext.EnsureLabel(ctx, testLabel)
		},
		Timeout: time.Minute,
		Fixture: "fixt3",
		Labels:  []string{testLabel},
	}}
	_ = runTestsAndReadAll(t, tests, cfg)

	for _, f := range []struct {
		name string
		fixt *recordingFixture
	}{
		{"1", &impl1},
		{"2", &impl2},
		{"3", &impl3},
	} {
		for _, step := range allFixtureSteps {
			if called, _ := f.fixt.called[step]; !called {
				t.Errorf("fixture %s step=%s not called", f.name, step)
			}
			if msg, panicked := f.fixt.msgs[step]; panicked {
				t.Errorf("fixture %s step=%s failed: %s", f.name, step, msg)
			}
		}
	}
}
