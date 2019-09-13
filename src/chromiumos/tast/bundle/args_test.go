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

var testFunc = func(context.Context, *testing.State) {}

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

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestCase(&testing.TestCase{Name: test2, Func: testFunc})
	testing.AddTestCase(&testing.TestCase{Name: test3, Func: testFunc})
	testing.AddTestCase(&testing.TestCase{Name: test1, Func: testFunc})

	stdin := newBufferWithArgs(t, &Args{Mode: RunTestsMode, RunTests: &RunTestsArgs{}})
	tests, err := readArgs(nil, stdin, ioutil.Discard, &Args{}, &runConfig{}, localBundle)
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

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestCase(&testing.TestCase{Name: name1, Func: testFunc, Timeout: customTimeout})
	testing.AddTestCase(&testing.TestCase{Name: name2, Func: testFunc})

	stdin := newBufferWithArgs(t, &Args{Mode: RunTestsMode, RunTests: &RunTestsArgs{}})
	tests, err := readArgs(nil, stdin, ioutil.Discard, &Args{},
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
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	const name = "cat.MyTest"
	testing.AddTestCase(&testing.TestCase{Name: name, Func: testFunc})

	// Adding a test with same name should generate an error.
	testing.AddTestCase(&testing.TestCase{Name: name, Func: testFunc})
	stdin := newBufferWithArgs(t, &Args{Mode: RunTestsMode, RunTests: &RunTestsArgs{}})
	if _, err := readArgs(nil, stdin, ioutil.Discard, &Args{},
		&runConfig{}, localBundle); !errorHasStatus(err, statusBadTests) {
		t.Errorf("readArgs() with bad test returned error %v; want status %v", err, statusBadTests)
	}
}
