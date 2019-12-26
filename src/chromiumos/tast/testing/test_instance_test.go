// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	gotesting "testing"
	"time"

	"chromiumos/tast/testutil"
)

// TESTINSTANCETEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_instance_test.go). The obvious choice, "TestInstanceTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "instance" and "test" are acronyms.
func TESTINSTANCETEST(context.Context, *State) {}

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

func TestAutoName(t *gotesting.T) {
	tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, nil)
	if err != nil {
		t.Fatal("failed to instantiate TestInstance: ", err)
	}
	if tc.Name != "testing.TESTINSTANCETEST" {
		t.Errorf("Unexpected test case name: got %s; want testing.TESTINSTANCETEST", tc.Name)
	}
}

func TestAutoAttr(t *gotesting.T) {
	test, err := newTestInstance(&Test{
		Func:         TESTINSTANCETEST,
		Attr:         []string{"attr1", "attr2"},
		SoftwareDeps: []string{"dep1", "dep2"},
	}, nil)
	if err != nil {
		t.Fatal("failed to instantiate test case failed: ", err)
	}
	exp := []string{
		"attr1",
		"attr2",
		testNameAttrPrefix + "testing.TESTINSTANCETEST",
		// The bundle name is the second-to-last component in the package's path.
		testBundleAttrPrefix + "tast",
		testDepAttrPrefix + "dep1",
		testDepAttrPrefix + "dep2",
	}
	sort.Strings(test.Attr)
	sort.Strings(exp)
	if !reflect.DeepEqual(test.Attr, exp) {
		t.Errorf("Attr = %v; want %v", test.Attr, exp)
	}
}

func TestAdditionalTime(t *gotesting.T) {
	pre := &testPre{}
	test, err := newTestInstance(&Test{Func: TESTINSTANCETEST, Timeout: 5 * time.Minute, Pre: pre}, nil)
	if err != nil {
		t.Fatal("finalize() failed: ", err)
	}
	if exp := preTestTimeout + postTestTimeout + 2*pre.Timeout(); test.AdditionalTime != exp {
		t.Errorf("AdditionalTime = %v; want %v", test.AdditionalTime, exp)
	}

	pre = &testPre{}
	if test, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, &Param{Timeout: time.Minute, Pre: pre}); err != nil {
		t.Error(err)
	} else if exp := preTestTimeout + postTestTimeout + 2*pre.Timeout(); test.AdditionalTime != exp {
		t.Errorf("AdditionalTime = %v; want %v", test.AdditionalTime, exp)
	}
}

func TestParamTest(t *gotesting.T) {
	test, err := newTestInstance(
		&Test{
			Func:         TESTINSTANCETEST,
			Attr:         []string{"attr1"},
			Data:         []string{"data1"},
			SoftwareDeps: []string{"dep1"},
		},
		&Param{
			Name:              "param1",
			Val:               10,
			ExtraAttr:         []string{"attr2"},
			ExtraData:         []string{"data2"},
			ExtraSoftwareDeps: []string{"dep2"},
		})
	if err != nil {
		t.Fatal("newTestInstance failed: ", err)
	}

	if test.Val != 10 {
		t.Errorf("Unexpected Val: got %v; want 10", test.Val)
	}

	if test.Name != "testing.TESTINSTANCETEST.param1" {
		t.Errorf("Unexpected name: got %s; want testing.TESTINSTANCETEST.param1", test.Name)
	}

	expectedAttr := []string{"name:testing.TESTINSTANCETEST.param1", "bundle:tast", "dep:dep1", "dep:dep2", "attr1", "attr2"}
	if !reflect.DeepEqual(test.Attr, expectedAttr) {
		t.Errorf("Unexpected attrs: got %v; want %v", test.Attr, expectedAttr)
	}

	expectedData := []string{"data1", "data2"}
	if !reflect.DeepEqual(test.Data, expectedData) {
		t.Errorf("Unexpected data: got %v; want %v", test.Data, expectedData)
	}

	expectedDeps := []string{"dep1", "dep2"}
	if !reflect.DeepEqual(test.SoftwareDeps, expectedDeps) {
		t.Errorf("Unexpected SoftwareDeps: got %v; want %v", test.SoftwareDeps, expectedDeps)
	}
}

func TestParamTestWithEmptyName(t *gotesting.T) {
	test, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, &Param{})
	if err != nil {
		t.Fatal("newTestInstance failed: ", err)
	}
	if test.Name != "testing.TESTINSTANCETEST" {
		t.Errorf("Unexpected name: got %s; want testing.TESTINSTANCETEST", test.Name)
	}
}

func TestParamTestWithPre(t *gotesting.T) {
	pre := &testPre{name: "precondition"}
	// At most one Pre condition can be present. If newTestInstance fails, test passes.
	if _, err := newTestInstance(&Test{Func: TESTINSTANCETEST, Pre: pre}, &Param{Pre: pre}); err == nil {
		t.Error("newTestInstance unexpectedly passed for duplicated preconditions")
	}

	// Precondition only at enclosing test.
	if tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST, Pre: pre}, &Param{}); err != nil {
		t.Error(err)
	} else if tc.Pre != pre {
		t.Errorf("Invalid precondition = %v; want %v", tc.Pre, pre)
	}

	// Precondition only at parametrized test.
	if tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, &Param{Pre: pre}); err != nil {
		t.Error(err)
	} else if tc.Pre != pre {
		t.Errorf("Invalid precondition = %v; want %v", tc.Pre, pre)
	}

	// No preconditions.
	if tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, &Param{}); err != nil {
		t.Error(err)
	} else if tc.Pre != nil {
		t.Errorf("Invalid precondition = %v; want nil", tc.Pre)
	}
}

func TestParamTestWithTimeout(t *gotesting.T) {
	// At most one Pre condition can be present. If newTestInstance fails, test passes.
	if _, err := newTestInstance(&Test{Func: TESTINSTANCETEST, Timeout: time.Minute}, &Param{Timeout: time.Minute}); err == nil {
		t.Error("newTestInstance unexpectedly passed for duplicated timeout")
	}

	// Timeout only at enclosing test.
	if tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST, Timeout: time.Minute}, &Param{}); err != nil {
		t.Error(err)
	} else if tc.Timeout != time.Minute {
		t.Errorf("Invalid Timeout = %v; want %v", tc.Timeout, time.Minute)
	}

	// Timeout only at parametrized test.
	if tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, &Param{Timeout: time.Minute}); err != nil {
		t.Error(err)
	} else if tc.Timeout != time.Minute {
		t.Errorf("Invalid precondition = %v; want %v", tc.Timeout, time.Minute)
	}

	// No Timeout.
	if tc, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, &Param{}); err != nil {
		t.Error(err)
	} else if tc.Timeout != 0 {
		t.Errorf("Invalid precondition = %v; want 0", tc.Timeout)
	}
}

func TestDataDir(t *gotesting.T) {
	test, err := newTestInstance(&Test{Func: TESTINSTANCETEST}, nil)
	if err != nil {
		t.Fatal(err)
	}
	exp := filepath.Join("chromiumos/tast/testing", testDataSubdir)
	if test.DataDir() != exp {
		t.Errorf("DataDir() = %q; want %q", test.DataDir(), exp)
	}
}

func TestSoftwareDeps(t *gotesting.T) {
	test := TestInstance{SoftwareDeps: []string{"dep3", "dep1", "dep2"}}
	missing := test.MissingSoftwareDeps([]string{"dep0", "dep2", "dep4"})
	if exp := []string{"dep1", "dep3"}; !reflect.DeepEqual(missing, exp) {
		t.Errorf("MissingSoftwareDeps() = %v; want %v", missing, exp)
	}
}

func TestRunSuccess(t *gotesting.T) {
	test := TestInstance{Func: func(context.Context, *State) {}, Timeout: time.Minute}
	or := newOutputReader()
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	od := filepath.Join(td, "out")
	test.Run(context.Background(), or.ch, &TestConfig{OutDir: od})
	if errs := getOutputErrors(or.read()); len(errs) != 0 {
		t.Errorf("Got unexpected error(s) for test: %v", errs)
	}
	if fi, err := os.Stat(od); err != nil {
		t.Errorf("Out dir %v not accessible after testing: %v", od, err)
	} else if mode := fi.Mode()&os.ModePerm | os.ModeSticky; mode != 0777|os.ModeSticky {
		t.Errorf("Out dir %v has mode 0%o; want 0%o", od, mode, 0777|os.ModeSticky)
	}
}

func TestRunPanic(t *gotesting.T) {
	test := TestInstance{Func: func(context.Context, *State) { panic("intentional panic") }, Timeout: time.Minute}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	if errs := getOutputErrors(or.read()); len(errs) != 1 {
		t.Errorf("Got %v errors for panicking test; want 1", errs)
	}
}

func TestRunDeadline(t *gotesting.T) {
	f := func(ctx context.Context, s *State) {
		// Wait for the context to report that the deadline has been hit.
		<-ctx.Done()
		s.Error("Saw timeout within test")
	}
	test := TestInstance{Func: f, Timeout: time.Millisecond, ExitTimeout: 10 * time.Second}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	// The error that was reported by the test after its deadline was hit
	// but within the exit delay should be available.
	if errs := getOutputErrors(or.read()); len(errs) != 1 {
		t.Errorf("Got %v errors for timed-out test; want 1", len(errs))
	}
}

func TestRunLogAfterTimeout(t *gotesting.T) {
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
	test := TestInstance{Func: f, Timeout: time.Millisecond, ExitTimeout: time.Millisecond}

	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})

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

func TestRunLateWriteFromGoroutine(t *gotesting.T) {
	// Run a test that calls s.Error from a goroutine after the test has finished.
	start := make(chan struct{}) // tells goroutine to start
	end := make(chan struct{})   // announces goroutine is done
	test := TestInstance{Func: func(ctx context.Context, s *State) {
		go func() {
			<-start
			s.Error("This message should be discarded since the test is done")
			close(end)
		}()
	}, Timeout: time.Minute}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})

	// Tell the goroutine to start and wait for it to finish.
	close(start)
	<-end

	// No errors should be reported, and we also shouldn't panic due to
	// the s.Error call trying to write to a closed channel.
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}
}

func TestRunSkipStages(t *gotesting.T) {
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
		test.Run(context.Background(), or.ch, cfg)

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

func TestRunMissingData(t *gotesting.T) {
	const (
		existingFile      = "existing.txt"
		missingFile1      = "missing1.txt"
		missingFile2      = "missing2.txt"
		missingErrorFile1 = missingFile1 + ExternalErrorSuffix
	)

	test := TestInstance{
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
	test.Run(context.Background(), or.ch, &TestConfig{DataDir: td})

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

func TestRunPrecondition(t *gotesting.T) {
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
	test.Run(context.Background(), or.ch, &TestConfig{})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}
}

func TestAttachStateToContext(t *gotesting.T) {
	test := TestInstance{
		Func: func(ctx context.Context, s *State) {
			ContextLog(ctx, "msg ", 1)
			ContextLogf(ctx, "msg %d", 2)
		},
		Timeout: time.Minute,
	}

	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	if out := or.read(); len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
	}
}

func TestJSON(t *gotesting.T) {
	orig := TestInstance{
		Func: TESTINSTANCETEST,
		Desc: "Description",
		Attr: []string{"attr1", "attr2"},
		Data: []string{"foo.txt"},
		Pkg:  "chromiumos/foo/bar",
	}
	b, err := json.Marshal(&orig)
	if err != nil {
		t.Fatalf("Failed to marshal %v: %v", orig, err)
	}
	loaded := TestInstance{}
	if err = json.Unmarshal(b, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal %s: %v", string(b), err)
	}

	// The function should be omitted.
	orig.Func = nil
	if !reflect.DeepEqual(loaded, orig) {
		t.Fatalf("Unmarshaled to %v; want %v", loaded, orig)
	}
}

func TestTestClone(t *gotesting.T) {
	const (
		name    = "OldTest"
		timeout = time.Minute
	)
	attr := []string{"a", "b"}
	softwareDeps := []string{"sw1", "sw2"}
	serviceDeps := []string{"svc1", "svc2"}
	f := func(context.Context, *State) {}

	// Checks that tst's fields still contain the above values.
	checkTest := func(msg string, tst *TestInstance) {
		if tst.Name != name {
			t.Errorf("%s set Name to %q; want %q", msg, tst.Name, name)
		}
		// Go doesn't allow checking functions for equality.
		if tst.Func == nil {
			t.Errorf("%s set Func to nil; want %p", msg, f)
		}
		if !reflect.DeepEqual(tst.Attr, attr) {
			t.Errorf("%s set Attr to %v; want %v", msg, tst.Attr, attr)
		}
		if !reflect.DeepEqual(tst.SoftwareDeps, softwareDeps) {
			t.Errorf("%s set SoftwareDeps to %v; want %v", msg, tst.SoftwareDeps, softwareDeps)
		}
		if !reflect.DeepEqual(tst.ServiceDeps, serviceDeps) {
			t.Errorf("%s set ServiceDeps to %v; want %v", msg, tst.ServiceDeps, serviceDeps)
		}
		if tst.Timeout != timeout {
			t.Errorf("%s set Timeout to %v; want %v", msg, tst.Timeout, timeout)
		}
	}

	// First check that a cloned copy gets the correct values.
	orig := TestInstance{
		Name:         name,
		Func:         f,
		Attr:         append([]string(nil), attr...),
		SoftwareDeps: append([]string(nil), softwareDeps...),
		ServiceDeps:  append([]string(nil), serviceDeps...),
		Timeout:      timeout,
	}
	clone := orig.clone()
	checkTest("clone()", clone)

	// Now update fields in the copy and check that the original is unaffected.
	clone.Name = "NewTest"
	clone.Func = nil
	clone.Attr[0] = "new"
	clone.Timeout = 2 * timeout
	clone.SoftwareDeps[0] = "swnew"
	clone.ServiceDeps[0] = "svcnew"
	checkTest("update after clone()", &orig)
}

func TestSortTests(t *gotesting.T) {
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}

	// Assign names with different leading digits to make sure we don't sort by name primarily.
	t1 := &TestInstance{Name: "3-test1", Pre: nil}
	t2 := &TestInstance{Name: "4-test2", Pre: nil}
	t3 := &TestInstance{Name: "1-test3", Pre: pre1}
	t4 := &TestInstance{Name: "2-test4", Pre: pre1}
	t5 := &TestInstance{Name: "0-test5", Pre: pre2}
	tests := []*TestInstance{t4, t2, t3, t5, t1}

	getNames := func(tests []*TestInstance) (names []string) {
		for _, test := range tests {
			names = append(names, test.Name)
		}
		return names
	}

	in := getNames(tests)
	SortTests(tests)
	actual := getNames(tests)
	expected := getNames([]*TestInstance{t1, t2, t3, t4, t5})
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Sort(%v) = %v; want %v", in, actual, expected)
	}
}
