// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"encoding/json"
	"reflect"
	gotesting "testing"
	"time"

	"chromiumos/tast/testing"
)

// newBufferWithArgs returns a bytes.Buffer containing the JSON representation of args.
func newBufferWithArgs(t *gotesting.T, args *Args) *bytes.Buffer {
	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(args); err != nil {
		t.Fatal(err)
	}
	return &b
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

	tests, err := readArgs(newBufferWithArgs(t, &Args{}), &Args{}, &runConfig{}, localBundle)
	if err != nil {
		t.Fatal("readArgs() failed: ", err)
	}
	var act []string
	for _, t := range tests {
		act = append(act, t.Name)
	}
	if exp := []string{test1, test2, test3}; !reflect.DeepEqual(act, exp) {
		t.Errorf("readArgs() returned tests %v; want sorted %v", act, exp)
	}
}

func TestReadArgsTestTimeouts(t *gotesting.T) {
	const (
		name1          = "pkg.Test1"
		name2          = "pkg.Test2"
		customTimeout  = 45 * time.Second
		defaultTimeout = 30 * time.Second
	)

	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: name1, Func: func(*testing.State) {}, Timeout: customTimeout})
	testing.AddTest(&testing.Test{Name: name2, Func: func(*testing.State) {}})

	tests, err := readArgs(newBufferWithArgs(t, &Args{}), &Args{},
		&runConfig{defaultTestTimeout: defaultTimeout}, localBundle)
	if err != nil {
		t.Fatal("readArgs() failed: ", err)
	}

	act := make(map[string]time.Duration)
	for _, t := range tests {
		act[t.Name] = t.Timeout
	}
	exp := map[string]time.Duration{name1: customTimeout, name2: defaultTimeout}
	if !reflect.DeepEqual(act, exp) {
		t.Errorf("Wanted tests/timeouts %v; got %v", act, exp)
	}
}

func TestReadArgsRegistrationError(t *gotesting.T) {
	defer testing.ClearForTesting()
	const name = "cat.MyTest"
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: name, Func: func(*testing.State) {}})

	// Adding a test without a function should generate an error.
	testing.AddTest(&testing.Test{})
	if _, err := readArgs(newBufferWithArgs(t, &Args{}), &Args{},
		&runConfig{}, localBundle); !errorHasStatus(err, statusBadTests) {
		t.Errorf("readArgs() with bad test returned error %v; want status %v", err, statusBadTests)
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
