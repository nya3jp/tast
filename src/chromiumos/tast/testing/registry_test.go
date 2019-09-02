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
func testsEqual(a, b []*TestCase) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Just do a basic comparison, since we clone tests and set additional attributes
		// when adding them to the registry.
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

// getDupeTestPtrs returns pointers present in both a and b.
func getDupeTestPtrs(a, b []*TestCase) []*TestCase {
	am := make(map[*TestCase]struct{}, len(a))
	for _, t := range a {
		am[t] = struct{}{}
	}
	var dupes []*TestCase
	for _, t := range b {
		if _, ok := am[t]; ok {
			dupes = append(dupes, t)
		}
	}
	return dupes
}

func TestAllTests(t *gotesting.T) {
	reg := NewRegistry()
	allTests := []*TestCase{
		&TestCase{Name: "test.Foo", Func: func(context.Context, *State) {}},
		&TestCase{Name: "test.Bar", Func: func(context.Context, *State) {}},
	}
	for _, test := range allTests {
		if err := reg.AddTestCase(test); err != nil {
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

func TestTestsForGlob(t *gotesting.T) {
	reg := NewRegistry()
	allTests := []*TestCase{
		&TestCase{Name: "test.Foo", Func: func(context.Context, *State) {}},
		&TestCase{Name: "test.Bar", Func: func(context.Context, *State) {}},
		&TestCase{Name: "blah.Foo", Func: func(context.Context, *State) {}},
	}
	for _, test := range allTests {
		if err := reg.AddTestCase(test); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		glob     string
		expected []*TestCase
	}{
		{"test.Foo", []*TestCase{allTests[0]}},
		{"test.Bar", []*TestCase{allTests[1]}},
		{"test.*", []*TestCase{allTests[0], allTests[1]}},
		{"*.Foo", []*TestCase{allTests[0], allTests[2]}},
		{"*.*", []*TestCase{allTests[0], allTests[1], allTests[2]}},
		{"*", []*TestCase{allTests[0], allTests[1], allTests[2]}},
		{"", []*TestCase{}},
		{"bogus", []*TestCase{}},
		// Test that periods are escaped.
		{"test.Fo.", []*TestCase{}},
	} {
		if tests, err := reg.testsForGlob(tc.glob); err != nil {
			t.Fatalf("testsForGlob(%q) failed: %v", tc.glob, err)
		} else if !testsEqual(tests, tc.expected) {
			t.Errorf("testsForGlob(%q) = %v; want %v", tc.glob, tests, tc.expected)
		}
	}

	// Now test multiple globs.
	for _, tc := range []struct {
		globs    []string
		expected []*TestCase
	}{
		{[]string{"test.Foo"}, []*TestCase{allTests[0]}},
		{[]string{"test.Foo", "test.Foo"}, []*TestCase{allTests[0]}},
		{[]string{"test.Foo", "test.Bar"}, []*TestCase{allTests[0], allTests[1]}},
		{[]string{"no.Matches"}, []*TestCase{}},
	} {
		if tests, err := reg.TestsForGlobs(tc.globs); err != nil {
			t.Fatalf("TestsForGlobs(%v) failed: %v", tc.globs, err)
		} else {
			if !testsEqual(tests, tc.expected) {
				t.Errorf("TestsForGlobs(%v) = %v; want %v", tc.globs, tests, tc.expected)
			}
			if dupes := getDupeTestPtrs(tests, tc.expected); len(dupes) != 0 {
				t.Errorf("TestsForGlobs(%v) returned non-copied test(s): %v", tc.globs, dupes)
			}
		}
	}
}

func TestTestsForAttrExpr(t *gotesting.T) {
	reg := NewRegistry()
	allTests := []*TestCase{
		&TestCase{Name: "test.Foo", Func: func(context.Context, *State) {}, Attr: []string{"test", "foo"}},
		&TestCase{Name: "test.Bar", Func: func(context.Context, *State) {}, Attr: []string{"test", "bar"}},
	}
	for _, test := range allTests {
		if err := reg.AddTestCase(test); err != nil {
			t.Fatal(err)
		}
	}

	// More-complicated expressions are tested by the attr package's tests.
	for _, tc := range []struct {
		expr     string
		expected []*TestCase
	}{
		{"foo", []*TestCase{allTests[0]}},
		{"bar", []*TestCase{allTests[1]}},
		{"test", allTests},
		{"test && !bar", []*TestCase{allTests[0]}},
		{"\"*est\"", allTests},
		{"\"*est\" && \"f*\"", []*TestCase{allTests[0]}},
		{"baz", []*TestCase{}},
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
	reg := NewRegistry()
	if err := reg.AddTestCase(&TestCase{Name: name, Func: func(context.Context, *State) {}}); err != nil {
		t.Fatal("Failed to add initial test: ", err)
	}
	if err := reg.AddTestCase(&TestCase{Name: name, Func: func(context.Context, *State) {}}); err == nil {
		t.Fatal("Duplicate test name unexpectedly not rejected")
	}
}

func TestAddTestModifyOriginal(t *gotesting.T) {
	reg := NewRegistry()
	const origName = "test.OldName"
	const origDep = "olddep"
	test := &TestCase{
		Name:         origName,
		Func:         func(context.Context, *State) {},
		SoftwareDeps: []string{origDep},
	}
	if err := reg.AddTestCase(test); err != nil {
		t.Fatal("AddTest failed: ", err)
	}

	// Change the original Test struct's name and modify the dependency slice's data.
	test.Name = "test.NewName"
	test.SoftwareDeps[0] = "newdep"

	// The test returned by the registry should still contain the original information.
	tests := reg.AllTests()
	if len(tests) != 1 {
		t.Fatalf("AllTests returned %v; wanted 1 test: ", tests)
	}
	if tests[0].Name != origName {
		t.Errorf("Test.Name is %q; want %q", tests[0].Name, origName)
	}
	if want := []string{origDep}; !reflect.DeepEqual(tests[0].SoftwareDeps, want) {
		t.Errorf("Test.Deps is %v; want %v", tests[0].SoftwareDeps, want)
	}
}

// Dummy function to register the test.
func RegistryTest(context.Context, *State) {}

func TestParamTestRegistration(t *gotesting.T) {
	reg := NewRegistry()
	test := &Test{
		Func: RegistryTest,
		Params: []Param{{
			Name: "param1",
		}, {
			Name: "param2",
		}},
	}

	if err := reg.AddTest(test); err != nil {
		t.Error("Unexpecetd test registration error: ", err)
	}

	// Makes sure the test is registered in order.
	tests := reg.AllTests()
	if len(tests) != 2 {
		t.Errorf("Unexpected number of registered tests: got %d; want 2", len(tests))
	}

	if tests[0].Name != "testing.RegistryTest.param1" {
		t.Errorf("Unexpected test name: got %s; want testing.RegistryTest.param1", tests[0].Name)
	}

	if tests[1].Name != "testing.RegistryTest.param2" {
		t.Errorf("Unexpected test name: got %s; want testing.RegistryTest.param2", tests[1].Name)
	}
}

func TestNewTestGlobRegexp(t *gotesting.T) {
	// Exact match case.
	if r, err := NewTestGlobRegexp("arc.Test"); err != nil {
		t.Error("Unexpected glob pattern error: ", err)
	} else {
		if !r.MatchString("arc.Test") {
			t.Error("Exact match didn't work")
		}
		if r.MatchString("arcXTest") {
			t.Error("Dot matched non-dot character unexpectedly")
		}
		if r.MatchString("fooarc.Test") {
			t.Error("Matched as suffix unexpectedly")
		}
		if r.MatchString("arc.TestFoo") {
			t.Error("Matched as prefix unexpectedly")
		}
	}

	// Glob pattern.
	if r, err := NewTestGlobRegexp("arc.*"); err != nil {
		t.Error("Unexpected glob pattern error: ", err)
	} else {
		if !r.MatchString("arc.Test") {
			t.Error("Glob didn't match")
		}
		if r.MatchString("arcXTest") {
			t.Error("Dot matched non-dot character unexpectedly")
		}
	}

	// Underscore is allowed for parameterized test.
	if r, err := NewTestGlobRegexp("arc.Test.param_1"); err != nil {
		t.Error("Unexpected glob pattern error: ", err)
	} else if !r.MatchString("arc.Test.param_1") {
		t.Error("Pattern with underscore didn't match with the name")
	}

	// Unexepcted glob pattern case.
	if _, err := NewTestGlobRegexp("arc.#*"); err == nil {
		t.Error("Glob pattern with '#' is successfully compiled unexpectedly")
	}
}
