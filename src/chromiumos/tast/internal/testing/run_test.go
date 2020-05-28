// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/testutil"
)

// testPre implements both Precondition and preconditionImpl for unit tests.
type testPre struct {
	prepareFunc func(context.Context, *State) interface{}
	closeFunc   func(context.Context, *State)
	name        string // name to return from String
}

func (p *testPre) Prepare(ctx context.Context, s *State) interface{} {
	if p.prepareFunc != nil {
		return p.prepareFunc(ctx, s)
	}
	return nil
}

func (p *testPre) Close(ctx context.Context, s *State) {
	if p.closeFunc != nil {
		p.closeFunc(ctx, s)
	}
}

func (p *testPre) Timeout() time.Duration { return time.Minute }

func (p *testPre) String() string { return p.name }

func TestRunSuccess(t *testing.T) {
	test := &TestInstance{Func: func(context.Context, *State) {}, Timeout: time.Minute}
	or := newOutputReader()
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	od := filepath.Join(td, "out")
	Run(context.Background(), test, or.ch, &TestConfig{OutDir: od})
	if errs := getOutputErrors(or.read()); len(errs) != 0 {
		t.Errorf("Got unexpected error(s) for test: %v", errs)
	}
	if fi, err := os.Stat(od); err != nil {
		t.Errorf("Out dir %v not accessible after testing: %v", od, err)
	} else if mode := fi.Mode()&os.ModePerm | os.ModeSticky; mode != 0777|os.ModeSticky {
		t.Errorf("Out dir %v has mode 0%o; want 0%o", od, mode, 0777|os.ModeSticky)
	}
}

func TestRunPanic(t *testing.T) {
	test := &TestInstance{Func: func(context.Context, *State) { panic("intentional panic") }, Timeout: time.Minute}
	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{})
	if errs := getOutputErrors(or.read()); len(errs) != 1 {
		t.Errorf("Got %v errors for panicking test; want 1", errs)
	}
}

func TestRunDeadline(t *testing.T) {
	f := func(ctx context.Context, s *State) {
		// Wait for the context to report that the deadline has been hit.
		<-ctx.Done()
		s.Error("Saw timeout within test")
	}
	test := &TestInstance{Func: f, Timeout: time.Millisecond, ExitTimeout: 10 * time.Second}
	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{})
	// The error that was reported by the test after its deadline was hit
	// but within the exit delay should be available.
	if errs := getOutputErrors(or.read()); len(errs) != 1 {
		t.Errorf("Got %v errors for timed-out test; want 1", len(errs))
	}
}

func TestRunLogAfterTimeout(t *testing.T) {
	cont := make(chan bool)
	done := make(chan bool)
	f := func(ctx context.Context, s *State) {
		// Report when we're done, either after completing or after panicking before completion.
		completed := false
		defer func() { done <- completed }()

		// Ignore the deadline and wait until we're told to continue.
		<-ctx.Done()
		<-cont
		s.Log("Done waiting")
		completed = true
	}
	test := &TestInstance{Func: f, Timeout: time.Millisecond, ExitTimeout: time.Millisecond}

	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{})

	// Tell the test to continue even though Run has already returned. The output channel should
	// still be open so as to avoid a panic when the test writes to it: https://crbug.com/853406
	cont <- true
	if completed := <-done; !completed {
		t.Error("Test function didn't complete")
	}
	// No output errors should be written to the channel; reporting timeouts is the caller's job.
	if errs := getOutputErrors(or.read()); len(errs) != 0 {
		t.Errorf("Got %v error(s) for runaway test; want 0", len(errs))
	}
}

func TestRunLateWriteFromGoroutine(t *testing.T) {
	// Run a test that calls s.Error from a goroutine after the test has finished.
	start := make(chan struct{}) // tells goroutine to start
	end := make(chan struct{})   // announces goroutine is done
	test := &TestInstance{Func: func(ctx context.Context, s *State) {
		go func() {
			<-start
			s.Error("This message should be discarded since the test is done")
			close(end)
		}()
	}, Timeout: time.Minute}
	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{})

	// Tell the goroutine to start and wait for it to finish.
	close(start)
	<-end

	// No errors should be reported, and we also shouldn't panic due to
	// the s.Error call trying to write to a closed channel.
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}
}

func TestRunSkipStages(t *testing.T) {
	type action int // actions that can be performed by stages
	const (
		pass    action = iota
		doError        // call State.Error
		doFatal        // call State.Fatal
		doPanic        // call panic()
		noCall         // stage should be skipped
	)

	// Define a sequence of tests to run and specify which stages should be executed for each.
	var pre, pre2, pre3, pre4 testPre
	cases := []struct {
		pre                *testPre
		preTestAction      action // TestConfig.PreTestFunc
		prepareAction      action // Precondition.Prepare
		testAction         action // Test.Func
		closeAction        action // Precondition.Close
		postTestAction     action // TestConfig.PostTestFunc
		postTestHookAction action // Return of TestConfig.PreTestFunc
		desc               string
	}{
		{&pre, pass, pass, pass, noCall, pass, pass, "everything passes"},
		{&pre, doError, noCall, noCall, noCall, pass, pass, "pre-test fails"},
		{&pre, doPanic, noCall, noCall, noCall, pass, pass, "pre-test panics"},
		{&pre, pass, doError, noCall, noCall, pass, pass, "prepare fails"},
		{&pre, pass, doPanic, noCall, noCall, pass, pass, "prepare panics"},
		{&pre, pass, pass, doError, noCall, pass, pass, "test fails"},
		{&pre, pass, pass, doPanic, noCall, pass, pass, "test panics"},
		{&pre, pass, pass, pass, pass, pass, pass, "everything passes, next test has different precondition"},
		{&pre2, pass, doError, noCall, pass, pass, pass, "prepare fails, next test has different precondition"},
		{&pre3, pass, pass, doError, pass, pass, pass, "test fails, next test has no precondition"},
		{nil, pass, noCall, pass, noCall, pass, pass, "no precondition"},
		{&pre4, pass, pass, pass, pass, pass, pass, "final test"},
	}

	// Create tests first so we can set TestConfig.NextTest later.
	var tests []*TestInstance
	for _, c := range cases {
		test := &TestInstance{Timeout: time.Minute}
		// We can't just do "test.Pre = c.pre" here. See e.g. https://tour.golang.org/methods/12:
		// "Note that an interface value that holds a nil concrete value is itself non-nil."
		if c.pre != nil {
			test.Pre = c.pre
		}
		tests = append(tests, test)
	}

	// makeFunc returns a function that sets *called to true and performs the action described by a.
	makeFunc := func(a action, called *bool) func(context.Context, *State) {
		return func(ctx context.Context, s *State) {
			*called = true
			switch a {
			case doError:
				s.Error("intentional error")
			case doFatal:
				s.Fatal("intentional fatal")
			case doPanic:
				panic("intentional panic")
			}
		}
	}

	makeFuncWithCallback := func(a action, called *bool, cbA action, cbCalled *bool) func(ctx context.Context, s *State) func(ctx context.Context, s *State) {
		return func(ctx context.Context, s *State) func(ctx context.Context, s *State) {
			*called = true
			switch a {
			case doError:
				s.Error("intentional error")
			case doFatal:
				s.Fatal("intentional fatal")
			case doPanic:
				panic("intentional panic")
			}

			return makeFunc(cbA, cbCalled)
		}
	}

	// Now actually run each test.
	for i, c := range cases {
		var preTestRan, prepareRan, testRan, closeRan, postTestRan, postTestHookRan bool

		test := tests[i]
		test.Func = makeFunc(c.testAction, &testRan)
		if c.pre != nil {
			prepare := makeFunc(c.prepareAction, &prepareRan)
			c.pre.prepareFunc = func(ctx context.Context, s *State) interface{} {
				prepare(ctx, s)
				return nil
			}
			c.pre.closeFunc = makeFunc(c.closeAction, &closeRan)
		}
		cfg := &TestConfig{
			PreTestFunc:  makeFuncWithCallback(c.preTestAction, &preTestRan, c.postTestHookAction, &postTestHookRan),
			PostTestFunc: makeFunc(c.postTestAction, &postTestRan),
		}
		if i < len(tests)-1 {
			cfg.NextTest = tests[i+1]
		}

		or := newOutputReader()
		Run(context.Background(), test, or.ch, cfg)

		// Verify that stages were executed or skipped as expected.
		checkRan := func(name string, ran bool, a action) {
			wantRun := a != noCall
			if !ran && wantRun {
				t.Errorf("Test %d (%s) didn't run %s", i, c.desc, name)
			} else if ran && !wantRun {
				t.Errorf("Test %d (%s) ran %s unexpectedly", i, c.desc, name)
			}
		}
		checkRan("TestConfig.PreTestFunc", preTestRan, c.preTestAction)
		checkRan("Precondition.Prepare", prepareRan, c.prepareAction)
		checkRan("Test.Func", testRan, c.testAction)
		checkRan("Precondition.Close", closeRan, c.closeAction)
		checkRan("TestConfig.PostTestFunc", postTestRan, c.postTestAction)
	}
}

func TestRunMissingData(t *testing.T) {
	const (
		existingFile      = "existing.txt"
		missingFile1      = "missing1.txt"
		missingFile2      = "missing2.txt"
		missingErrorFile1 = missingFile1 + ExternalErrorSuffix
	)

	test := &TestInstance{
		Func:    func(context.Context, *State) {},
		Data:    []string{existingFile, missingFile1, missingFile2},
		Timeout: time.Minute,
	}

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	if err := ioutil.WriteFile(filepath.Join(td, existingFile), nil, 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", existingFile, err)
	}
	if err := ioutil.WriteFile(filepath.Join(td, missingErrorFile1), []byte("some reason"), 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", missingErrorFile1, err)
	}

	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{DataDir: td})

	expected := []string{
		"Required data file missing1.txt missing: some reason",
		"Required data file missing2.txt missing",
	}
	if errs := getOutputErrors(or.read()); len(errs) != 2 {
		t.Errorf("Got %v errors for missing data test; want 2", errs)
	} else if actual := []string{errs[0].Reason, errs[1].Reason}; !reflect.DeepEqual(actual, expected) {
		t.Errorf("Got errors %q; want %q", actual, expected)
	}
}

func TestRunPrecondition(t *testing.T) {
	type data struct{}
	preData := &data{}

	// The test should be able to access the data via State.PreValue.
	test := &TestInstance{
		// Use a precondition that returns the struct that we declared earlier from its Prepare method.
		Pre: &testPre{
			prepareFunc: func(context.Context, *State) interface{} { return preData },
		},
		Func: func(ctx context.Context, s *State) {
			if s.PreValue() == nil {
				s.Fatal("Precondition value not supplied")
			} else if d, ok := s.PreValue().(*data); !ok {
				s.Fatal("Precondition value didn't have expected type")
			} else if d != preData {
				s.Fatalf("Got precondition value %v; want %v", d, preData)
			}
		},
		Timeout: time.Minute,
	}

	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}
}

func TestRunPreconditionContext(t *testing.T) {
	var testCtx context.Context

	prepareFunc := func(ctx context.Context, s *State) interface{} {
		if testCtx == nil {
			testCtx = s.PreCtx()
		}

		if testCtx != s.PreCtx() {
			t.Errorf("Different context in Prepare")
		}

		if _, ok := ContextSoftwareDeps(s.PreCtx()); !ok {
			t.Error("ContextSoftwareDeps unavailable")
		}
		return nil
	}

	closeFunc := func(ctx context.Context, s *State) {
		if testCtx != s.PreCtx() {
			t.Errorf("Different context in Close")
		}
	}

	testFunc := func(ctx context.Context, s *State) {
		defer func() {
			expectedPanic := "PreCtx can only be called in a precondition"

			if r := recover(); r == nil {
				t.Errorf("PreCtx did not panic")
			} else if r != expectedPanic {
				t.Errorf("PreCtx unexpected panic: want %q; got %q", expectedPanic, r)
			}
		}()

		s.PreCtx()
	}

	pre := &testPre{
		prepareFunc: prepareFunc,
		closeFunc:   closeFunc,
	}

	t1 := &TestInstance{Name: "t1", Pre: pre, Timeout: time.Minute, Func: testFunc}
	t2 := &TestInstance{Name: "t2", Pre: pre, Timeout: time.Minute, Func: testFunc}

	or := newOutputReader()
	Run(context.Background(), t1, or.ch, &TestConfig{
		NextTest: t2,
	})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}

	if t1.PreCtx != t2.PreCtx {
		t.Errorf("PreCtx different between test instances")
	}

	or = newOutputReader()
	Run(context.Background(), t2, or.ch, &TestConfig{
		NextTest: nil,
	})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}

	if t1.PreCtx.Err() == nil {
		t.Errorf("Context not cancelled")
	}
}

func TestAttachStateToContext(t *testing.T) {
	test := &TestInstance{
		Func: func(ctx context.Context, s *State) {
			logging.ContextLog(ctx, "msg ", 1)
			logging.ContextLogf(ctx, "msg %d", 2)
		},
		Timeout: time.Minute,
	}

	or := newOutputReader()
	Run(context.Background(), test, or.ch, &TestConfig{})
	if out := or.read(); len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
	}
}
