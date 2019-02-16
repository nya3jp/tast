// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	gotesting "testing"
)

// testsEqual returns true if a and b contain tests with matching fields.
// This is useful when comparing slices that contain copies of the same underlying tests.
func testsEqual(a, b []*Test) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ta := *a[i]
		tb := *b[i]

		// Clear functions, as reflect.DeepEqual always returns false for non-nil Func types.
		ta.Func = nil
		tb.Func = nil

		if !reflect.DeepEqual(ta, tb) {
			return false
		}
	}
	return true
}

// getDupeTestPtrs returns pointers present in both a and b.
func getDupeTestPtrs(a, b []*Test) []*Test {
	am := make(map[*Test]struct{}, len(a))
	for _, t := range a {
		am[t] = struct{}{}
	}
	var dupes []*Test
	for _, t := range b {
		if _, ok := am[t]; ok {
			dupes = append(dupes, t)
		}
	}
	return dupes
}

func TestAllTests(t *gotesting.T) {
	reg := NewRegistry(NoAutoName)
	allTests := []*Test{
		&Test{Name: "test.Foo", Func: func(context.Context, *State) {}},
		&Test{Name: "test.Bar", Func: func(context.Context, *State) {}},
	}
	for _, test := range allTests {
		if err := reg.AddTest(test); err != nil {
			t.Fatal(err)
		}
	}

	tests := reg.AllTests()
	if !testsEqual(tests, allTests) {
		t.Errorf("AllTests() = %v; want %v", tests, allTests)
	}
	if dupes := getDupeTestPtrs(tests, allTests); len(dupes) != 0 {
		t.Errorf("AllTests() returned non-copied test(s): %v", dupes)
	}
}

func TestTestsForPattern(t *gotesting.T) {
	reg := NewRegistry(NoAutoName)
	allTests := []*Test{
		&Test{Name: "test.Foo", Func: func(context.Context, *State) {}},
		&Test{Name: "test.Bar", Func: func(context.Context, *State) {}},
		&Test{Name: "blah.Foo", Func: func(context.Context, *State) {}},
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
		} else {
			if !testsEqual(tests, tc.expected) {
				t.Errorf("TestsForPatterns(%v) = %v; want %v", tc.pats, tests, tc.expected)
			}
			if dupes := getDupeTestPtrs(tests, tc.expected); len(dupes) != 0 {
				t.Errorf("TestsForPatterns(%v) returned non-copied test(s): %v", tc.pats, dupes)
			}
		}
	}
}

func TestTestsForAttrExpr(t *gotesting.T) {
	reg := NewRegistry(NoAutoName)
	allTests := []*Test{
		&Test{Name: "test.Foo", Func: func(context.Context, *State) {}, Attr: []string{"test", "foo"}},
		&Test{Name: "test.Bar", Func: func(context.Context, *State) {}, Attr: []string{"test", "bar"}},
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
		{"test", allTests},
		{"test && !bar", []*Test{allTests[0]}},
		{"\"*est\"", allTests},
		{"\"*est\" && \"f*\"", []*Test{allTests[0]}},
		{"baz", []*Test{}},
	} {
		tests, err := reg.TestsForAttrExpr(tc.expr)
		if err != nil {
			t.Errorf("TestsForAttrExpr(%v) failed: %v", tc.expr, err)
		} else {
			if !testsEqual(tests, tc.expected) {
				t.Errorf("TestsForAttrExpr(%q) = %v; want %v", tc.expr, tests, tc.expected)
			}
			if dupes := getDupeTestPtrs(tests, tc.expected); len(dupes) != 0 {
				t.Errorf("TestsForAttrExpr(%q) returned non-copied test(s): %v", tc.expr, dupes)
			}
		}
	}
}

func TestAddTestDuplicateName(t *gotesting.T) {
	const name = "test.Foo"
	reg := NewRegistry(NoAutoName)
	if err := reg.AddTest(&Test{Name: name, Func: func(context.Context, *State) {}}); err != nil {
		t.Fatal("Failed to add initial test: ", err)
	}
	if err := reg.AddTest(&Test{Name: name, Func: func(context.Context, *State) {}}); err == nil {
		t.Fatal("Duplicate test name unexpectedly not rejected")
	}
}
