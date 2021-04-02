// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"testing"
)

func TestMatcher(t *testing.T) {
	for _, tc := range []struct {
		pats []string
		want bool
	}{
		{[]string{}, true},
		{[]string{""}, false},
		{[]string{"pkg.Test"}, true},
		{[]string{"pkg.Test2"}, false},
		{[]string{"xpkg.Test"}, false},
		{[]string{"pkg.Tes"}, false},
		{[]string{"kg.Test"}, false},
		{[]string{"pkg.*"}, true},
		{[]string{"foo.*"}, false},
		{[]string{"*.Test"}, true},
		{[]string{"*.Bar"}, false},
		{[]string{"*.*"}, true},
		{[]string{"*.Tes."}, false}, // ensure dots are escaped
		{[]string{"(attr1)"}, true},
		{[]string{"(attr2)"}, false},
		{[]string{"(!attr1)"}, false},
		{[]string{"(!attr2)"}, true},
		{[]string{"(attr1 || attr2)"}, true},
		{[]string{"(attr1 && attr2)"}, false},
		{[]string{`("attr1")`}, true},
		{[]string{`("a*1")`}, true},
	} {
		m, err := NewMatcher(tc.pats)
		if err != nil {
			t.Fatalf("Failed to compile %q: %v", tc.pats, err)
		}
		if got := m.Match("pkg.Test", []string{"attr1"}); got != tc.want {
			t.Errorf("Result mismatch for %q: got %v, want %v", tc.pats, got, tc.want)
		}
	}
}

func TestMatcherBadPatterns(t *testing.T) {
	for _, pat := range []string{
		"[]",
		"(",
		"test-Fo.",
	} {
		if _, err := NewMatcher([]string{pat}); err == nil {
			t.Errorf("NewMatcher unexpectedly succeeded for %q", pat)
		}
	}
}
