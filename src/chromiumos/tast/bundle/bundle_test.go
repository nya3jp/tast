// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	gotesting "testing"
	"time"

	"chromiumos/tast/control"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

// newBufferWithArgs returns a bytes.Buffer containing the JSON representation of args.
func newBufferWithArgs(t *gotesting.T, args *Args) *bytes.Buffer {
	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(args); err != nil {
		t.Fatal(err)
	}
	return &b
}

// callReadArgs calls readArgs with the supplied arguments.
// It returns readArgs's return values, along with a buffer containing the resulting
// stdout output and a function signature that can be included in test failure messages.
func callReadArgs(t *gotesting.T, stdinArgs *Args, defaultArgs *Args, bt bundleType) (
	cfg *runConfig, status int, stdout *bytes.Buffer, sig string) {
	stdin := newBufferWithArgs(t, stdinArgs)
	stdout = &bytes.Buffer{}
	cfg, status = readArgs(stdin, stdout, defaultArgs, bt)
	sig = fmt.Sprintf("readArgs(%+v, stdout, %+v, %v)", stdinArgs, defaultArgs, bt)
	return cfg, status, stdout, sig
}

func TestReadArgsSortTests(t *gotesting.T) {
	const (
		test1 = "pkg.Test1"
		test2 = "pkg.Test2"
		test3 = "pkg.Test3"
	)

	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: test2, Func: func(*testing.State) {}})
	testing.AddTest(&testing.Test{Name: test3, Func: func(*testing.State) {}})
	testing.AddTest(&testing.Test{Name: test1, Func: func(*testing.State) {}})

	cfg, _, _, sig := callReadArgs(t, &Args{}, &Args{}, localBundle)
	if cfg == nil {
		t.Fatalf("%v returned nil config", sig)
	}
	var act []string
	for _, t := range cfg.tests {
		act = append(act, t.Name)
	}
	if exp := []string{test1, test2, test3}; !reflect.DeepEqual(act, exp) {
		t.Errorf("%v returned tests %v; want sorted %v", sig, act, exp)
	}
}

func TestReadArgsList(t *gotesting.T) {
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: "pkg.Test", Func: func(*testing.State) {}})

	cfg, status, stdout, sig := callReadArgs(t, &Args{Mode: ListTestsMode}, &Args{}, localBundle)
	if status != statusSuccess {
		t.Fatalf("%v returned status %v; want %v", sig, status, statusSuccess)
	}
	if cfg != nil {
		t.Errorf("%s returned non-nil config %+v", sig, cfg)
	}
	var exp bytes.Buffer
	if err := testing.WriteTestsAsJSON(&exp, testing.GlobalRegistry().AllTests()); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != exp.String() {
		t.Errorf("%s wrote %q; want %q", sig, stdout.String(), exp.String())
	}
}

func TestReadArgsRegistrationError(t *gotesting.T) {
	defer testing.ClearForTesting()
	const name = "cat.MyTest"
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: name, Func: func(*testing.State) {}})

	// Adding a test without a function should generate an error.
	testing.AddTest(&testing.Test{})

	if _, status, _, sig := callReadArgs(t, &Args{}, &Args{}, localBundle); status != statusBadTests {
		t.Fatalf("%v returned status %v; want %v", sig, status, statusBadTests)
	}
}

func TestTestsToRun(t *gotesting.T) {
	const (
		name1 = "cat.MyTest1"
		name2 = "cat.MyTest2"
	)
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: name1, Func: func(*testing.State) {}, Attr: []string{"attr1", "attr2"}})
	testing.AddTest(&testing.Test{Name: name2, Func: func(*testing.State) {}, Attr: []string{"attr2"}})

	for _, tc := range []struct {
		args     []string
		expNames []string // expected test names, or nil if error is expected
	}{
		{[]string{}, []string{name1, name2}},
		{[]string{name1}, []string{name1}},
		{[]string{name2, name1}, []string{name2, name1}},
		{[]string{"cat.*"}, []string{name1, name2}},
		{[]string{"(attr1)"}, []string{name1}},
		{[]string{"(attr2)"}, []string{name1, name2}},
		{[]string{"(!attr1)"}, []string{name2}},
		{[]string{"(attr1 || attr2)"}, []string{name1, name2}},
		{[]string{""}, []string{}},
		{[]string{"("}, nil},
		{[]string{"()"}, nil},
		{[]string{"attr1 || attr2"}, nil},
		{[]string{"(attr3)"}, []string{}},
		{[]string{"foo.BogusTest"}, []string{}},
	} {
		tests, err := testsToRun(tc.args)
		if tc.expNames == nil {
			if err == nil {
				t.Errorf("testsToRun(%v) succeeded unexpectedly", tc.args)
			}
			continue
		}

		if err != nil {
			t.Errorf("testsToRun(%v) failed: %v", tc.args, err)
		} else {
			actNames := make([]string, len(tests))
			for i := range tests {
				actNames[i] = tests[i].Name
			}
			if !reflect.DeepEqual(actNames, tc.expNames) {
				t.Errorf("testsToRun(%v) = %v; want %v", tc.args, actNames, tc.expNames)
			}
		}
	}
}

func TestCopyTestOutput(t *gotesting.T) {
	const msg = "here is a log message"
	e := testing.Error{
		Reason: "something went wrong",
		File:   "file.go",
		Line:   16,
		Stack:  "the stack",
	}
	t1 := time.Unix(1, 0)
	t2 := time.Unix(2, 0)

	ch := make(chan testing.Output)
	go func() {
		ch <- testing.Output{T: t1, Msg: msg}
		ch <- testing.Output{T: t2, Err: &e}
		close(ch)
	}()

	b := bytes.Buffer{}
	copyTestOutput(ch, control.NewMessageWriter(&b))

	r := control.NewMessageReader(&b)
	for i, em := range []interface{}{
		&control.TestLog{Time: t1, Text: msg},
		&control.TestError{Time: t2, Error: e},
	} {
		if am, err := r.ReadMessage(); err != nil {
			t.Errorf("Failed to read message %v: %v", i, err)
		} else if !reflect.DeepEqual(am, em) {
			t.Errorf("Message %v is %v; want %v", i, am, em)
		}
	}
	if r.More() {
		t.Error("copyTestOutput() wrote extra message(s)")
	}
}

func TestRunTests(t *gotesting.T) {
	const (
		name1 = "foo.Test1"
		name2 = "foo.Test2"
	)

	reg := testing.NewRegistry()
	reg.DisableValidationForTesting()
	reg.AddTest(&testing.Test{Name: name1, Func: func(*testing.State) {}})
	reg.AddTest(&testing.Test{Name: name2, Func: func(s *testing.State) { s.Error("error") }})

	tmpDir := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(tmpDir)

	b := bytes.Buffer{}
	numSetupCalls := 0
	cfg := runConfig{
		args: &Args{
			OutDir:  tmpDir,
			DataDir: tmpDir,
		},
		mw:        control.NewMessageWriter(&b),
		tests:     reg.AllTests(),
		setupFunc: func() error { numSetupCalls++; return nil },
	}

	if status := runTests(context.Background(), &cfg); status != statusSuccess {
		t.Fatalf("RunTests(%v) = %v; want %v", cfg, status, statusSuccess)
	}
	if numSetupCalls != len(cfg.tests) {
		t.Errorf("RunTests(%v) called setup function %d time(s); want %d", cfg, numSetupCalls, len(cfg.tests))
	}

	// Just check some basic details of the control messages.
	r := control.NewMessageReader(&b)
	for i, ei := range []interface{}{
		&control.TestStart{Test: *cfg.tests[0]},
		&control.TestEnd{Name: name1},
		&control.TestStart{Test: *cfg.tests[1]},
		&control.TestError{},
		&control.TestEnd{Name: name2},
	} {
		if ai, err := r.ReadMessage(); err != nil {
			t.Errorf("Failed to read message %d: %v", i, err)
		} else {
			switch em := ei.(type) {
			case *control.TestStart:
				if am, ok := ai.(*control.TestStart); !ok {
					t.Errorf("Got %v at %d; want TestStart", ai, i)
				} else if am.Test.Name != em.Test.Name {
					t.Errorf("Got TestStart with Test %q at %d; want %q", am.Test.Name, i, em.Test.Name)
				}
			case *control.TestEnd:
				if am, ok := ai.(*control.TestEnd); !ok {
					t.Errorf("Got %v at %d; want TestEnd", ai, i)
				} else if am.Name != em.Name {
					t.Errorf("Got TestEnd for %q at %d; want %q", am.Name, i, em.Name)
				}
			case *control.TestError:
				if _, ok := ai.(*control.TestError); !ok {
					t.Errorf("Got %v at %d; want TestError", ai, i)
				}
			}
		}
	}
	if r.More() {
		t.Errorf("RunTests(%v) wrote extra message(s)", cfg)
	}
}

func TestTimeout(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.DisableValidationForTesting()

	// The first test blocks indefinitely on a channel.
	const name1 = "foo.Test1"
	ch := make(chan bool, 1)
	defer func() { ch <- true }()
	reg.AddTest(&testing.Test{
		Name:           name1,
		Func:           func(*testing.State) { <-ch },
		CleanupTimeout: time.Millisecond, // avoid blocking after timeout
	})

	// The second test blocks for 50 ms and specifies a custom one-minute timeout.
	const name2 = "foo.Test2"
	reg.AddTest(&testing.Test{
		Name:    name2,
		Func:    func(*testing.State) { time.Sleep(50 * time.Millisecond) },
		Timeout: time.Minute,
	})

	b := bytes.Buffer{}
	tmpDir := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(tmpDir)
	cfg := runConfig{
		args: &Args{
			OutDir:  tmpDir,
			DataDir: tmpDir,
		},
		mw:                 control.NewMessageWriter(&b),
		tests:              reg.AllTests(),
		defaultTestTimeout: 10 * time.Millisecond,
	}

	// The first test should time out after 10 milliseconds.
	// The second test should succeed since it finishes before its custom timeout.
	if status := runTests(context.Background(), &cfg); status != statusSuccess {
		t.Fatalf("RunTests(ctx, %v) = %v; want %v", cfg, status, statusSuccess)
	}

	var name string             // name of current test
	errors := make([]string, 0) // name of test from each error
	r := control.NewMessageReader(&b)
	for r.More() {
		if msg, err := r.ReadMessage(); err != nil {
			t.Error("ReadMessage failed: ", err)
		} else if ts, ok := msg.(*control.TestStart); ok {
			name = ts.Test.Name
		} else if _, ok := msg.(*control.TestError); ok {
			errors = append(errors, name)
		}
	}
	exp := []string{name1}
	if !reflect.DeepEqual(errors, exp) {
		t.Errorf("Got errors %v; wanted %v", errors, exp)
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	// runTests should report failure when passed a config without any tests.
	cfg := runConfig{tests: nil}
	if status := runTests(context.Background(), &cfg); status != statusNoTests {
		t.Fatalf("RunTests(ctx, %v) = %v; want %v", cfg, status, statusNoTests)
	}
}
