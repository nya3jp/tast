// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"reflect"
	gotesting "testing"
	"time"

	"chromiumos/tast/testing"
)

var testFunc func(context.Context, *testing.State) = func(context.Context, *testing.State) {}

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

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	testing.AddTest(&testing.Test{Name: test2, Func: testFunc})
	testing.AddTest(&testing.Test{Name: test3, Func: testFunc})
	testing.AddTest(&testing.Test{Name: test1, Func: testFunc})

	tests, err := readArgs(nil, newBufferWithArgs(t, &Args{}), ioutil.Discard,
		&Args{}, &runConfig{}, localBundle)
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

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	testing.AddTest(&testing.Test{Name: name1, Func: testFunc, Timeout: customTimeout})
	testing.AddTest(&testing.Test{Name: name2, Func: testFunc})

	tests, err := readArgs(nil, newBufferWithArgs(t, &Args{}), ioutil.Discard, &Args{},
		&runConfig{defaultTestTimeout: defaultTimeout}, localBundle)
	if err != nil {
		t.Fatal("readArgs() failed: ", err)
	}

	act := make(map[string]time.Duration, len(tests))
	for _, t := range tests {
		act[t.Name] = t.Timeout
	}
	exp := map[string]time.Duration{name1: customTimeout, name2: defaultTimeout}
	if !reflect.DeepEqual(act, exp) {
		t.Errorf("Wanted tests/timeouts %v; got %v", act, exp)
	}
}

func TestReadArgsRegistrationError(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	const name = "cat.MyTest"
	testing.AddTest(&testing.Test{Name: name, Func: testFunc})

	// Adding a test without a function should generate an error.
	testing.AddTest(&testing.Test{})
	if _, err := readArgs(nil, newBufferWithArgs(t, &Args{}), ioutil.Discard, &Args{},
		&runConfig{}, localBundle); !errorHasStatus(err, statusBadTests) {
		t.Errorf("readArgs() with bad test returned error %v; want status %v", err, statusBadTests)
	}
}

func TestTestsToRun(t *gotesting.T) {
	const (
		name1 = "cat.MyTest1"
		name2 = "cat.MyTest2"
	)
	reg := testing.NewRegistry(testing.NoAutoName)
	reg.AddTest(&testing.Test{Name: name1, Func: testFunc, Attr: []string{"attr1", "attr2"}})
	reg.AddTest(&testing.Test{Name: name2, Func: testFunc, Attr: []string{"attr2"}})

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
		tests, err := TestsToRun(reg, tc.args)
		if tc.expNames == nil {
			if err == nil {
				t.Errorf("TestsToRun(..., %v) succeeded unexpectedly", tc.args)
			}
			continue
		}

		if err != nil {
			t.Errorf("TestsToRun(..., %v) failed: %v", tc.args, err)
		} else {
			actNames := make([]string, len(tests))
			for i := range tests {
				actNames[i] = tests[i].Name
			}
			if !reflect.DeepEqual(actNames, tc.expNames) {
				t.Errorf("TestsToRun(..., %v) = %v; want %v", tc.args, actNames, tc.expNames)
			}
		}
	}
}
