// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"reflect"
	gotesting "testing"
	"time"

	"chromiumos/tast/common/control"
	"chromiumos/tast/common/testing"
)

func MyFunc(*testing.State) {}

func TestTestsToRun(t *gotesting.T) {
	const (
		name1 = "cat.MyTest1"
		name2 = "cat.MyTest2"
	)
	defer testing.ClearForTesting()
	testing.AddTest(&testing.Test{Name: name1, Func: MyFunc, Attr: []string{"attr1", "attr2"}})
	testing.AddTest(&testing.Test{Name: name2, Func: MyFunc, Attr: []string{"attr2"}})

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
	testing.AddTest(&testing.Test{Name: name, Func: MyFunc})

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
	if CopyTestOutput(ch, w) {
		t.Error("CopyTestOutput() reported success for failed test")
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
		t.Error("CopyTestOutput() wrote extra message(s)")
	}
}
