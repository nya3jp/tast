// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	gotesting "testing"
	"time"

	"chromiumos/tast/testutil"
)

// Func1 is an arbitrary public test function used by unit tests.
func Func1(context.Context, *State) {}

// testPre implements Precondition for unit tests.
type testPre struct {
	prepareFunc, closeFunc func(context.Context, *State)

	name string
}

func (p *testPre) Prepare(ctx context.Context, s *State) {
	if p.prepareFunc != nil {
		p.prepareFunc(ctx, s)
	}
}

func (p *testPre) Close(ctx context.Context, s *State) {
	if p.closeFunc != nil {
		p.closeFunc(ctx, s)
	}
}

func (p *testPre) Timeout() time.Duration { return time.Minute }

func (p *testPre) String() string { return p.name }

func TestMissingFunc(t *gotesting.T) {
	test := Test{Name: "category.MyName"}
	if err := test.finalize(false); err == nil {
		t.Error("Didn't get error with missing function")
	}
}

func TestInvalidTestName(t *gotesting.T) {
	test := Test{Name: "Invalid%@!", Func: Func1}
	if err := test.finalize(false); err == nil {
		t.Error("Didn't get error with invalid name")
	}
}

func TestNegativeTimeout(t *gotesting.T) {
	test := Test{Name: "cat.Name", Func: Func1, Timeout: -1 * time.Second}
	if err := test.finalize(false); err == nil {
		t.Error("Didn't get error with negative timeout")
	}
}

func TestValidateDataPath(t *gotesting.T) {
	test := Test{Name: "cat.Name", Func: Func1, Data: []string{"foo", "bar/baz"}}
	if err := test.finalize(false); err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}
}

func TestValidateDataPathUnclean(t *gotesting.T) {
	test := Test{Name: "cat.Name", Func: Func1, Data: []string{"foo", "bar/../bar/baz"}}
	if err := test.finalize(false); err == nil {
		t.Error("Did not get an error with unclean path")
	}
}

func TestValidateDataPathAbsolutePath(t *gotesting.T) {
	test := Test{Name: "cat.Name", Func: Func1, Data: []string{"foo", "/etc/passwd"}}
	if err := test.finalize(false); err == nil {
		t.Error("Did not get an error with absolute path")
	}
}

func TestValidateDataPathRelativePath(t *gotesting.T) {
	test := Test{Name: "cat.Name", Func: Func1, Data: []string{"foo", "../baz"}}
	if err := test.finalize(false); err == nil {
		t.Error("Did not get an error with relative path")
	}
}

// TESTTEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_test.go). The obvious choice, "TestTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "test" is an acronym.
func TESTTEST(context.Context, *State) {}

func TestAutoName(t *gotesting.T) {
	test := Test{Func: TESTTEST}
	if err := test.finalize(true); err != nil {
		t.Error("Got error when finalizing test with valid test func name: ", err)
	} else if exp := "testing.TESTTEST"; test.Name != exp {
		t.Errorf("Test was given name %q; want %q", test.Name, exp)
	}
}

func TestAutoNameInvalid(t *gotesting.T) {
	test := Test{Func: Func1}
	if err := test.finalize(true); err == nil {
		t.Error("Didn't get expected error when finalizing test with invalid test func name")
	}
}

func TestAutoAttr(t *gotesting.T) {
	test := Test{
		Name:         "category.Name",
		Func:         Func1,
		Attr:         []string{"attr1", "attr2"},
		SoftwareDeps: []string{"dep1", "dep2"},
	}
	if err := test.finalize(false); err != nil {
		t.Fatal("finalize failed: ", err)
	}
	exp := []string{
		"attr1",
		"attr2",
		testNameAttrPrefix + "category.Name",
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

func TestReservedAttrPrefixes(t *gotesting.T) {
	test := Test{Name: "cat.Test", Func: Func1}
	for _, attr := range []string{
		testNameAttrPrefix + "foo",
		testBundleAttrPrefix + "bar",
		testDepAttrPrefix + "dep",
	} {
		test.Attr = []string{attr}
		if err := test.finalize(false); err == nil {
			t.Errorf("finalize didn't return error for reserved attribute %q", attr)
		}
	}
}

func TestAdditionalTime(t *gotesting.T) {
	pre := &testPre{}
	test := Test{Name: "cat.Name", Func: Func1, Timeout: 5 * time.Minute, Pre: pre}
	if err := test.finalize(false); err != nil {
		t.Error("finalize() failed: ", err)
	}
	if exp := setupFuncTimeout + cleanupFuncTimeout + 2*pre.Timeout(); test.AdditionalTime != exp {
		t.Errorf("AdditionalTime = %v; want %v", test.AdditionalTime, exp)
	}
}

func TestDataDir(t *gotesting.T) {
	test := Test{Name: "cat.Name", Func: Func1}
	if err := test.finalize(false); err != nil {
		t.Fatal(err)
	}
	exp := filepath.Join("chromiumos/tast/testing", testDataSubdir)
	if test.DataDir() != exp {
		t.Errorf("DataDir() = %q; want %q", test.DataDir(), exp)
	}
}

func TestSoftwareDeps(t *gotesting.T) {
	test := Test{Func: Func1, SoftwareDeps: []string{"dep3", "dep1", "dep2"}}
	missing := test.MissingSoftwareDeps([]string{"dep0", "dep2", "dep4"})
	if exp := []string{"dep1", "dep3"}; !reflect.DeepEqual(missing, exp) {
		t.Errorf("MissingSoftwareDeps() = %v; want %v", missing, exp)
	}
}

func TestRunSuccess(t *gotesting.T) {
	test := Test{Func: func(context.Context, *State) {}, Timeout: time.Minute}
	or := newOutputReader()
	td := testutil.TempDir(t)
	od := filepath.Join(td, "out")
	test.Run(context.Background(), or.ch, &TestConfig{OutDir: od})
	if errs := getOutputErrors(or.read()); len(errs) != 0 {
		t.Errorf("Got unexpected error(s) for test: %v", errs)
	}
	if _, err := os.Stat(od); err != nil {
		t.Errorf("Out dir %v not accessible after testing: %v", od, err)
	}
}

func TestRunPanic(t *gotesting.T) {
	test := Test{Func: func(context.Context, *State) { panic("intentional panic") }, Timeout: time.Minute}
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
	test := Test{Func: f, Timeout: time.Millisecond, ExitTimeout: 10 * time.Second}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	// The error that was reported by the test after its deadline was hit
	// but within the cleanup delay should be available.
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
	test := Test{Func: f, Timeout: time.Millisecond, ExitTimeout: time.Millisecond}

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
		pre           *testPre
		setupAction   action // TestConfig.SetupFunc
		prepareAction action // Precondition.Prepare
		testAction    action // Test.Func
		closeAction   action // Precondition.Close
		cleanupAction action // TestConfig.CleanupFunc
		desc          string
	}{
		{&pre, pass, pass, pass, noCall, pass, "everything passes"},
		{&pre, doError, noCall, noCall, noCall, noCall, "setup fails"},
		{&pre, doPanic, noCall, noCall, noCall, noCall, "setup panics"},
		{&pre, pass, doError, noCall, noCall, noCall, "prepare fails"},
		{&pre, pass, doPanic, noCall, noCall, noCall, "prepare panics"},
		{&pre, pass, pass, doError, noCall, pass, "test fails"},
		{&pre, pass, pass, doPanic, noCall, pass, "test panics"},
		{&pre, pass, pass, pass, pass, pass, "everything passes, next test has different precondition"},
		{&pre2, pass, doError, noCall, pass, noCall, "prepare fails, next test has different precondition"},
		{&pre3, pass, pass, doError, pass, pass, "test fails, next test has no precondition"},
		{nil, pass, noCall, pass, noCall, pass, "no precondition"},
		{&pre4, pass, pass, pass, pass, pass, "final test"},
	}

	// Create tests first so we can set TestConfig.NextTest later.
	var tests []*Test
	for _, c := range cases {
		test := &Test{Timeout: time.Minute}
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

	// Now actually run each test.
	for i, c := range cases {
		var setupRan, prepareRan, testRan, closeRan, cleanupRan bool

		test := tests[i]
		test.Func = makeFunc(c.testAction, &testRan)
		if c.pre != nil {
			c.pre.prepareFunc = makeFunc(c.prepareAction, &prepareRan)
			c.pre.closeFunc = makeFunc(c.closeAction, &closeRan)
		}
		cfg := &TestConfig{
			SetupFunc:   makeFunc(c.setupAction, &setupRan),
			CleanupFunc: makeFunc(c.cleanupAction, &cleanupRan),
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
		checkRan("TestConfig.SetupFunc", setupRan, c.setupAction)
		checkRan("Precondition.Prepare", prepareRan, c.prepareAction)
		checkRan("Test.Func", testRan, c.testAction)
		checkRan("Precondition.Close", closeRan, c.closeAction)
		checkRan("TestConfig.CleanupFunc", cleanupRan, c.cleanupAction)
	}
}

func TestAttachStateToContext(t *gotesting.T) {
	test := Test{
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
	orig := Test{
		Func: Func1,
		Desc: "Description",
		Attr: []string{"attr1", "attr2"},
		Data: []string{"foo.txt"},
		Pkg:  "chromiumos/foo/bar",
	}
	b, err := json.Marshal(&orig)
	if err != nil {
		t.Fatalf("Failed to marshal %v: %v", orig, err)
	}
	loaded := Test{}
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
	f := func(context.Context, *State) {}

	// Checks that tst's fields still contain the above values.
	checkTest := func(msg string, tst *Test) {
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
		if tst.SoftwareDeps != nil {
			t.Errorf("%s set SoftwareDeps to %v; want nil", msg, tst.SoftwareDeps)
		}
		if tst.Timeout != timeout {
			t.Errorf("%s set Timeout to %v; want %v", msg, tst.Timeout, timeout)
		}
	}

	// First check that a cloned copy gets the correct values.
	orig := Test{
		Name:    name,
		Func:    f,
		Attr:    append([]string{}, attr...),
		Timeout: timeout,
	}
	clone := orig.clone()
	checkTest("clone()", clone)

	// Now update fields in the copy and check that the original is unaffected.
	clone.Name = "NewTest"
	clone.Func = nil
	clone.Attr[0] = "new"
	clone.Timeout = 2 * timeout
	checkTest("update after clone()", &orig)
}

func TestCheckFuncNameAgainstFilename(t *gotesting.T) {
	for _, tc := range []struct {
		name, fn string
		valid    bool
	}{
		{"Test", "test.go", true},                     // single word
		{"MyTest", "my_test.go", true},                // two words separated with underscores
		{"LoadURL", "load_url.go", true},              // word and acronym
		{"PlayMP3", "play_mp3.go", true},              // word contains numbers
		{"PlayMP3Song", "play_mp3_song.go", true},     // acronym followed by word
		{"ConnectToDBus", "connect_to_dbus.go", true}, // word with multiple leading caps
		{"RestartCrosVM", "restart_crosvm.go", true},  // word with ending acronym
		{"RestartCrosVM", "restart_cros_vm.go", true}, // word followed by acronym
		{"Foo123bar", "foo123bar.go", true},           // word contains digits
		{"Foo123Bar", "foo123_bar.go", true},          // word with trailing digits
		{"Foo123bar", "foo_123bar.go", true},          // word with leading digits
		{"Foo123Bar", "foo_123_bar.go", true},         // word consisting only of digits
		{"foo", "foo.go", false},                      // lowercase func name
		{"Foobar", "foo_bar.go", false},               // lowercase word
		{"FirstTest", "first.go", false},              // func name has word not in filename
		{"Firstblah", "first.go", false},              // func name has word longer than filename
		{"First", "firstabc.go", false},               // filename has word longer than func name
		{"First", "first_test.go", false},             // filename has word not in func name
		{"FooBar", "foo__bar.go", false},              // empty word in filename
		{"Foo", "bar.go", false},                      // completely different words
		{"Foo", "Foo.go", false},                      // non-lowercase filename
		{"Foo", "foo.txt", false},                     // filename without ".go" extension
	} {
		err := checkFuncNameAgainstFilename(tc.name, tc.fn)
		if err != nil && tc.valid {
			t.Fatalf("checkFuncNameAgainstFilename(%q, %q) failed: %v", tc.name, tc.fn, err)
		} else if err == nil && !tc.valid {
			t.Fatalf("checkFuncNameAgainstFilename(%q, %q) didn't return expected error", tc.name, tc.fn)
		}
	}
}

func TestSortTests(t *gotesting.T) {
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}

	// Assign names with different leading digits to make sure we don't sort by name primarily.
	t1 := &Test{Name: "3-test1", Pre: nil}
	t2 := &Test{Name: "4-test2", Pre: nil}
	t3 := &Test{Name: "1-test3", Pre: pre1}
	t4 := &Test{Name: "2-test4", Pre: pre1}
	t5 := &Test{Name: "0-test5", Pre: pre2}
	tests := []*Test{t4, t2, t3, t5, t1}

	getNames := func(tests []*Test) (names []string) {
		for _, test := range tests {
			names = append(names, test.Name)
		}
		return names
	}

	in := getNames(tests)
	SortTests(tests)
	actual := getNames(tests)
	expected := getNames([]*Test{t1, t2, t3, t4, t5})
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Sort(%v) = %v; want %v", in, actual, expected)
	}
}
