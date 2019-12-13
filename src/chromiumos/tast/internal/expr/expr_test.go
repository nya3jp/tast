// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package expr

import (
	"fmt"
	"strings"
	"testing"
)

func TestGoodExpr(t *testing.T) {
	for _, tc := range []struct {
		expr, attrs string
		expMatch    bool
	}{
		{"a", "a", true},
		{"a", "", false},
		{"a", "b", false},
		{"a_b", "a_b", true},
		{"a || b || c", "a", true},
		{"a || b || c", "b", true},
		{"a || b || c", "c", true},
		{"a || b || c", "d", false},
		{"(a || b) && !c", "a", true},
		{"(a || b) && !c", "a d", true},
		{"(a || b) && !c", "a c", false},

		// quoted attributes
		{"c", "a:b c", true},
		{"\"c\"", "a:b c", true},
		{"\"a:b\"", "a:b c", true},
		{"\"a:b\"", "a", false},
		{"\"a:*b*\"", "a:b", true},
		{"\"a:*b*\"", "a:cbd", true},
		{"\"a:*b*\"", "a:", false},
	} {
		e, err := New(tc.expr)
		if err != nil {
			t.Errorf("New(%q) failed: %v", tc.expr, err)
		}
		if actMatch := e.Matches(strings.Fields(tc.attrs)); actMatch != tc.expMatch {
			t.Errorf("%q Matches(%q) = %v; want %v", tc.expr, tc.attrs, actMatch, tc.expMatch)
		}
	}
}

func TestBadExpr(t *testing.T) {
	for _, s := range []string{
		"",
		"a b",
		"a + b",
		"a == b",
		"(a && b",
		"a:b",
	} {
		if _, err := New(s); err == nil {
			t.Errorf("New(%q) unexpectedly succeeded", s)
		}
	}
}

func ExampleExpr() {
	e, _ := New("a && (b || c) && !d")
	for _, attrs := range [][]string{
		[]string{"a"},
		[]string{"a", "b"},
		[]string{"a", "c"},
		[]string{"a", "c", "d"},
	} {
		if e.Matches(attrs) {
			fmt.Println(attrs, "matched")
		} else {
			fmt.Println(attrs, "not matched")
		}
	}
	// Output:
	// [a] not matched
	// [a b] matched
	// [a c] matched
	// [a c d] not matched
}

func ExampleExpr_quoted() {
	e, _ := New("\"attr-hyphen\" && \"attr space\" && attr_under")
	if e.Matches([]string{"attr-hyphen", "attr space", "attr_under"}) {
		fmt.Println("matched")
	}
	// Output: matched
}

func ExampleExpr_wildcard() {
	e, _ := New("\"foo:*\" && !\"*bar\"")
	for _, attrs := range [][]string{
		[]string{"foo:"},
		[]string{"foo:a"},
		[]string{"foo:a", "bar"},
		[]string{"foo:a", "foo:bar"},
	} {
		if e.Matches(attrs) {
			fmt.Println(attrs, "matched")
		} else {
			fmt.Println(attrs, "not matched")
		}
	}
	// Output:
	// [foo:] matched
	// [foo:a] matched
	// [foo:a bar] not matched
	// [foo:a foo:bar] not matched
}
