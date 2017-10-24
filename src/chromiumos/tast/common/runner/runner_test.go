// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"context"
	"os"
	"reflect"
	gotesting "testing"
	"time"

	"chromiumos/tast/common/control"
	"chromiumos/tast/common/testing"
	"chromiumos/tast/common/testutil"
)

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
		{[]string{""}, nil},
		{[]string{"("}, nil},
		{[]string{"()"}, nil},
		{[]string{"attr1 || attr2"}, nil},
		{[]string{"(attr3)"}, nil},
		{[]string{"foo.BogusTest"}, nil},
	} {
		tests, err := TestsToRun(tc.args)
		if tc.expNames == nil {
			if err == nil {
				t.Errorf("TestsToRun(%v) succeeded unexpectedly", tc.args)
			}
			continue
		}

		if err != nil {
			t.Errorf("TestsToRun(%v) failed: %v", tc.args, err)
		} else {
			actNames := make([]string, len(tests))
			for i := range tests {
				actNames[i] = tests[i].Name
			}
			if !reflect.DeepEqual(actNames, tc.expNames) {
				t.Errorf("TestsToRun(%v) = %v; want %v", tc.args, actNames, tc.expNames)
			}
		}
	}
}

func TestTestsToRunRegistrationError(t *gotesting.T) {
	defer testing.ClearForTesting()
	const name = "cat.MyTest"
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: name, Func: func(*testing.State) {}})

	// Adding a test without a function should generate an error.
	testing.AddTest(&testing.Test{})

	if _, err := TestsToRun([]string{name}); err == nil {
		t.Errorf("TestsToRun(%q) didn't return registration error", name)
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
	w := control.NewMessageWriter(&b)
	if copyTestOutput(ch, w) {
		t.Error("copyTestOutput() reported success for failed test")
	}

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
	reg.AddTest(&testing.Test{Name: name2, Func: func(s *testing.State) { s.Errorf("error") }})

	tmpDir := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(tmpDir)

	b := bytes.Buffer{}
	numSetupCalls := 0
	cfg := RunConfig{
		Ctx:           context.Background(),
		Tests:         reg.AllTests(),
		MessageWriter: control.NewMessageWriter(&b),
		SetupFunc:     func() error { numSetupCalls++; return nil },
		BaseOutDir:    tmpDir,
		DataDir:       tmpDir,
	}

	numFailed, err := RunTests(cfg)
	if err != nil {
		t.Fatalf("RunTests(%v) failed: %v", cfg, err)
	}
	if numFailed != 1 {
		t.Fatalf("RunTests(%v) reported %d test failure(s); want 1", cfg, numFailed)
	}
	if numSetupCalls != len(cfg.Tests) {
		t.Errorf("RunTests(%v) called setup function %d time(s); want %d", cfg, numSetupCalls, len(cfg.Tests))
	}

	// Just check some basic details of the control messages.
	r := control.NewMessageReader(&b)
	for i, ei := range []interface{}{
		&control.TestStart{Name: name1, Test: *cfg.Tests[0]},
		&control.TestEnd{Name: name1},
		&control.TestStart{Name: name2, Test: *cfg.Tests[1]},
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
				} else {
					if am.Name != em.Name {
						t.Errorf("Got TestStart for %q at %d; want %q", am.Name, i, em.Name)
					}
					if am.Test.Name != em.Test.Name {
						t.Errorf("Got TestStart with Test %q at %d; want %q", am.Test.Name, i, em.Test.Name)
					}
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
	reg.AddTest(&testing.Test{Name: name1, Func: func(*testing.State) { <-ch }})

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
	cfg := RunConfig{
		Ctx:                context.Background(),
		Tests:              reg.AllTests(),
		MessageWriter:      control.NewMessageWriter(&b),
		BaseOutDir:         tmpDir,
		DataDir:            tmpDir,
		DefaultTestTimeout: 10 * time.Millisecond,
	}

	// The first test should time out after 10 milliseconds.
	// The second test should succeed since it finishes before its custom timeout.
	if numFailed, err := RunTests(cfg); err != nil {
		t.Fatal("RunTests failed: ", err)
	} else if numFailed != 1 {
		t.Errorf("RunTests reported %v failed; want 1", numFailed)
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
