// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package attr

import (
	"strings"
	"testing"
)

func TestGoodExpr(t *testing.T) {
	for _, tc := range []struct {
		expr, attr string
		expMatch   bool
	}{
		{"a", "a", true},
		{"a", "", false},
		{"a", "b", false},
		{"a || b || c", "a", true},
		{"a || b || c", "b", true},
		{"a || b || c", "c", true},
		{"a || b || c", "d", false},
		{"(a || b) && !c", "a", true},
		{"(a || b) && !c", "a d", true},
		{"(a || b) && !c", "a c", false},
	} {
		e, err := NewExpr(tc.expr)
		if err != nil {
			t.Errorf("NewExpr(%q) failed: %v", tc.expr, err)
		}
		if actMatch := e.Matches(strings.Fields(tc.attr)); actMatch != tc.expMatch {
			t.Errorf("%q Matches(%q) = %v; want %v", tc.expr, tc.attr, actMatch, tc.expMatch)
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
	} {
		if _, err := NewExpr(s); err == nil {
			t.Errorf("NewExpr(%q) unexpectedly succeeded", s)
		}
	}
}
