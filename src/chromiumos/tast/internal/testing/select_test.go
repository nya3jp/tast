// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSelectTestsByArgs(t *testing.T) {
	const (
		name1 = "cat.MyTest1"
		name2 = "cat.MyTest2"
	)
	allTests := []*TestInstance{
		{Name: name1, Func: func(context.Context, *State) {}, Attr: []string{"attr1", "attr2"}},
		{Name: name2, Func: func(context.Context, *State) {}, Attr: []string{"attr2"}},
	}

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
		tests, err := SelectTestsByArgs(allTests, tc.args)
		if tc.expNames == nil {
			if err == nil {
				t.Errorf("SelectTestsByArgs(..., %v) succeeded unexpectedly", tc.args)
			}
			continue
		}

		if err != nil {
			t.Errorf("SelectTestsByArgs(..., %v) failed: %v", tc.args, err)
		} else {
			actNames := make([]string, len(tests))
			for i := range tests {
				actNames[i] = tests[i].Name
			}
			sort.Strings(actNames)
			sort.Strings(tc.expNames)
			if !reflect.DeepEqual(actNames, tc.expNames) {
				t.Errorf("SelectTestsByArgs(..., %v) = %v; want %v", tc.args, actNames, tc.expNames)
			}
		}
	}
}

func TestSelectTestsByGlobs(t *testing.T) {
	allTests := []*TestInstance{
		{Name: "test.Foo", Func: func(context.Context, *State) {}},
		{Name: "test.Bar", Func: func(context.Context, *State) {}},
		{Name: "blah.Foo", Func: func(context.Context, *State) {}},
	}
	testNamesToInsts := make(map[string](*TestInstance))
	for _, t := range allTests {
		testNamesToInsts[t.Name] = t
	}

	for _, tc := range []struct {
		glob     string
		expected []*TestInstance
	}{
		{"test.Foo", []*TestInstance{allTests[0]}},
		{"test.Bar", []*TestInstance{allTests[1]}},
		{"test.*", []*TestInstance{allTests[0], allTests[1]}},
		{"*.Foo", []*TestInstance{allTests[0], allTests[2]}},
		{"*.*", []*TestInstance{allTests[0], allTests[1], allTests[2]}},
		{"*", []*TestInstance{allTests[0], allTests[1], allTests[2]}},
		{"", []*TestInstance{}},
		{"bogus", []*TestInstance{}},
		// Test that periods are escaped.
		{"test.Fo.", []*TestInstance{}},
	} {
		tests, err := selectTestsByGlob(testNamesToInsts, tc.glob)
		if err != nil {
			t.Fatalf("selectTestsByGlob(%q) failed: %v", tc.glob, err)
		}
		sort.Slice(tests, func(i, j int) bool {
			return tests[i].Name > tests[j].Name
		})
		sort.Slice(tc.expected, func(i, j int) bool {
			return tests[i].Name > tests[j].Name
		})
		if !testsEqual(tests, tc.expected) {
			t.Errorf("selectTestsByGlob(%q) = %v; want %v", tc.glob, tests, tc.expected)
		}
	}

	// Now test multiple globs.
	for _, tc := range []struct {
		globs    []string
		expected []*TestInstance
	}{
		{[]string{"test.Foo"}, []*TestInstance{allTests[0]}},
		{[]string{"test.Foo", "test.Foo"}, []*TestInstance{allTests[0]}},
		{[]string{"test.Foo", "test.Bar"}, []*TestInstance{allTests[0], allTests[1]}},
		{[]string{"no.Matches"}, []*TestInstance{}},
	} {
		if tests, err := SelectTestsByGlobs(allTests, tc.globs); err != nil {
			t.Fatalf("SelectTestsByGlobs(%v) failed: %v", tc.globs, err)
		} else {
			if !testsEqual(tests, tc.expected) {
				t.Errorf("SelectTestsByGlobs(%v) = %v; want %v", tc.globs, tests, tc.expected)
			}
			if dupes := getDupeTestPtrs(tests, tc.expected); len(dupes) != 0 {
				t.Errorf("SelectTestsByGlobs(%v) returned non-copied test(s): %v", tc.globs, dupes)
			}
		}
	}
}

func TestSelectTestsByAttrExpr(t *testing.T) {
	allTests := []*TestInstance{
		{Name: "test.Foo", Func: func(context.Context, *State) {}, Attr: []string{"test", "foo"}},
		{Name: "test.Bar", Func: func(context.Context, *State) {}, Attr: []string{"test", "bar"}},
	}

	// More-complicated expressions are tested by the attr package's tests.
	for _, tc := range []struct {
		expr     string
		expected []*TestInstance
	}{
		{"foo", []*TestInstance{allTests[0]}},
		{"bar", []*TestInstance{allTests[1]}},
		{"test", allTests},
		{"test && !bar", []*TestInstance{allTests[0]}},
		{"\"*est\"", allTests},
		{"\"*est\" && \"f*\"", []*TestInstance{allTests[0]}},
		{"baz", []*TestInstance{}},
	} {
		tests, err := SelectTestsByAttrExpr(allTests, tc.expr)
		if err != nil {
			t.Errorf("SelectTestsByAttrExpr(%v) failed: %v", tc.expr, err)
		} else {
			if !testsEqual(tests, tc.expected) {
				t.Errorf("SelectTestsByAttrExpr(%q) = %v; want %v", tc.expr, tests, tc.expected)
			}
			if dupes := getDupeTestPtrs(tests, tc.expected); len(dupes) != 0 {
				t.Errorf("SelectTestsByAttrExpr(%q) returned non-copied test(s): %v", tc.expr, dupes)
			}
		}
	}
}

func TestNewTestGlobRegexp(t *testing.T) {
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

// TestHasWildcardInTestPattern tests the function hasWildcardInTestPattern
func TestHasWildcardInTestPattern(t *testing.T) {
	for _, tc := range []struct {
		pat           string
		expectedError bool
	}{
		{"test.Foo", false},
		{"test_Bar.Foo", false},
		{"test.*", false},
		{"*.Foo", false},
		{"*.*", false},
		{"*", false},
		{"*_*", false},
		{"[]", true},
		{"(", true},
		{"test-Fo.", true},
	} {
		hasWildcard, err := hasWildcardInTestPattern(tc.pat)
		if err != nil {
			if !tc.expectedError {
				t.Errorf("hasWildcardInTestPattern(%q) failed: %v", tc.pat, err)
			}
			continue
		}
		if tc.expectedError {
			t.Errorf("hasWildcardInTestPattern(%q) passed unexpectedly", tc.pat)
		}
		if strings.Contains(tc.pat, "*") != hasWildcard {
			t.Errorf("hasWildcardInTestPattern(%q) returned %v; expected %v", tc.pat, hasWildcard, !hasWildcard)
		}
	}
}

// TestSelectTestsByGlobsScale makes sure run around O(NlogN)
func TestSelectTestsByGlobsScale(t *testing.T) {
	const acceptableRatio = 1.5 // Ratio for O(N^2) is 2 and ratio for O(N) is 1.
	const smallSampleSize = 160000
	const largeSampleSize = 320000
	var tests [largeSampleSize]*TestInstance
	var patterns [largeSampleSize]string

	for i := 0; i < largeSampleSize; i++ {
		patterns[i] = fmt.Sprintf("%06d", i)
		tests[i] = &TestInstance{Name: patterns[i]}
	}
	rand.Shuffle(largeSampleSize, func(i, j int) { tests[i], tests[j] = tests[j], tests[i] })

	start := time.Now()
	if _, err := SelectTestsByGlobs(tests[0:largeSampleSize], patterns[0:smallSampleSize]); err != nil {
		t.Fatal("SelectTestsByGlobs for small sample failed: ", err)
	}
	smallSampleTime := time.Duration(time.Since(start)).Seconds()

	rand.Shuffle(largeSampleSize, func(i, j int) { patterns[i], patterns[j] = patterns[j], patterns[i] })
	start = time.Now()
	if _, err := SelectTestsByGlobs(tests[0:largeSampleSize], patterns[0:largeSampleSize]); err != nil {
		t.Fatal("SelectTestsByGlobs for big sample failed: ", err)
	}
	largeSampleTime := time.Duration(time.Since(start)).Seconds()

	ratio := largeSampleTime / smallSampleTime
	logRatio := math.Log2(ratio)

	if logRatio > acceptableRatio {
		t.Fatalf("SelectTestsByGlobs runtime log ratio %v is above limit %v: ", logRatio, acceptableRatio)
	}

}
