// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package expr_test

import (
	"fmt"
	"strings"
	"testing"

	"chromiumos/tast/internal/expr"
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
		e, err := expr.New(tc.expr)
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
		if _, err := expr.New(s); err == nil {
			t.Errorf("expr.New(%q) unexpectedly succeeded", s)
		}
	}
}

func ExampleExpr() {
	e, _ := expr.New("a && (b || c) && !d")
	for _, attrs := range [][]string{
		{"a"},
		{"a", "b"},
		{"a", "c"},
		{"a", "c", "d"},
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
	e, _ := expr.New("\"attr-hyphen\" && \"attr space\" && attr_under")
	if e.Matches([]string{"attr-hyphen", "attr space", "attr_under"}) {
		fmt.Println("matched")
	}
	// Output: matched
}

func ExampleExpr_wildcard() {
	e, _ := expr.New("\"foo:*\" && !\"*bar\"")
	for _, attrs := range [][]string{
		{"foo:"},
		{"foo:a"},
		{"foo:a", "bar"},
		{"foo:a", "foo:bar"},
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
