// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"reflect"
	gotesting "testing"
	"time"
)

func TestTestsForPattern(t *gotesting.T) {
	reg := NewRegistry()
	allTests := []*Test{
		&Test{Name: "test.Foo", Func: Func1},
		&Test{Name: "test.Bar", Func: Func1},
		&Test{Name: "blah.Foo", Func: Func1},
	}
	for _, test := range allTests {
		if err := reg.AddTest(test); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		pat      string
		expected []*Test
	}{
		{"test.Foo", []*Test{allTests[0]}},
		{"test.Bar", []*Test{allTests[1]}},
		{"test.*", []*Test{allTests[0], allTests[1]}},
		{"*.Foo", []*Test{allTests[0], allTests[2]}},
		{"*.*", []*Test{allTests[0], allTests[1], allTests[2]}},
		{"*", []*Test{allTests[0], allTests[1], allTests[2]}},
		{"", []*Test{}},
		{"bogus", []*Test{}},
		// Test that periods are escaped.
		{"test.Fo.", []*Test{}},
	} {
		if tests, err := reg.testsForPattern(tc.pat); err != nil {
			t.Fatalf("testsForPattern(%q) failed: %v", tc.pat, err)
		} else if !reflect.DeepEqual(tests, tc.expected) {
			t.Errorf("testsForPattern(%q) = %v; want %v", tc.pat, tests, tc.expected)
		}
	}

	// Now test multiple patterns.
	for _, tc := range []struct {
		pats     []string
		expected []*Test
	}{
		{[]string{"test.Foo"}, []*Test{allTests[0]}},
		{[]string{"test.Foo", "test.Foo"}, []*Test{allTests[0]}},
		{[]string{"test.Foo", "test.Bar"}, []*Test{allTests[0], allTests[1]}},
		{[]string{"no.Matches"}, []*Test{}},
	} {
		if tests, err := reg.TestsForPatterns(tc.pats); err != nil {
			t.Fatalf("TestsForPatterns(%v) failed: %v", tc.pats, err)
		} else if !reflect.DeepEqual(tests, tc.expected) {
			t.Errorf("TestsForPatterns(%v) = %v; want %v", tc.pats, tests, tc.expected)
		}
	}
}

func TestTestsForAttrExpr(t *gotesting.T) {
	reg := NewRegistry()
	allTests := []*Test{
		&Test{Name: "test.Foo", Func: Func1, Attr: []string{"test", "foo"}},
		&Test{Name: "test.Bar", Func: Func1, Attr: []string{"test", "bar"}},
	}
	for _, test := range allTests {
		if err := reg.AddTest(test); err != nil {
			t.Fatal(err)
		}
	}

	// More-complicated expressions are tested by the attr package's tests.
	for _, tc := range []struct {
		expr     string
		expected []*Test
	}{
		{"foo", []*Test{allTests[0]}},
		{"bar", []*Test{allTests[1]}},
		{"test", []*Test{allTests[0], allTests[1]}},
		{"test && !bar", []*Test{allTests[0]}},
		{"baz", []*Test{}},
	} {
		tests, err := reg.TestsForAttrExpr(tc.expr)
		if err != nil {
			t.Errorf("TestsForAttrExpr(%v) failed: %v", tc.expr, err)
		} else if !reflect.DeepEqual(tests, tc.expected) {
			t.Errorf("TestsForAttrExpr(%v) = %v; want %v", tc.expr, tests, tc.expected)
		}
	}
}

func TestAddTestFailsForInvalidTests(t *gotesting.T) {
	reg := NewRegistry()

	if err := reg.AddTest(&Test{Name: "Invalid%@!", Func: Func1}); err == nil {
		t.Errorf("Didn't get error when adding test with invalid name")
	}
	if err := reg.AddTest(&Test{Name: "pkg.MissingFunc"}); err == nil {
		t.Errorf("Didn't get error when adding test with missing function")
	}
	if err := reg.AddTest(&Test{Func: func(*State) {}}); err == nil {
		t.Errorf("Didn't get error when adding test with unexported function")
	}
	if err := reg.AddTest(&Test{Func: Func1, Timeout: -1 * time.Second}); err == nil {
		t.Errorf("Didn't get error when adding test with negative timeout")
	}
}
