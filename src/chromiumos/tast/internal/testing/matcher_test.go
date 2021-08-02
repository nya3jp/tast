// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing_test

import (
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/testing"
)

func TestMatcher(t *gotesting.T) {
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
		m, err := testing.NewMatcher(tc.pats)
		if err != nil {
			t.Fatalf("Failed to compile %q: %v", tc.pats, err)
		}
		if got := m.Match("pkg.Test", []string{"attr1"}); got != tc.want {
			t.Errorf("Result mismatch for %q: got %v, want %v", tc.pats, got, tc.want)
		}
	}
}

func TestMatcherBadPatterns(t *gotesting.T) {
	for _, pat := range []string{
		"[]",
		"(",
		"test-Fo.",
	} {
		if _, err := testing.NewMatcher([]string{pat}); err == nil {
			t.Errorf("NewMatcher unexpectedly succeeded for %q", pat)
		}
	}
}

func TestMatcherUnmatchedPatternsNames(t *gotesting.T) {
	// Test a combination of all, none, and some tests found, mixed with wildcards and expressions.
	for _, tc := range []struct {
		tests []string
		pats  []string
		want  []string
	}{
		{
			tests: []string{"example.Pass"},
			pats:  []string{"foo.bar", "example.Pass"},
			want:  []string{"foo.bar"},
		},
		{
			tests: []string{"example.Pass"},
			pats:  []string{"foo.bar", "example.Pass", "bar.foo"},
			want:  []string{"bar.foo", "foo.bar"},
		},
		{
			tests: []string{},
			pats:  []string{"foo.bar", "example.Pass", "bar.foo"},
			want:  []string{"bar.foo", "example.Pass", "foo.bar"},
		},
		{
			tests: []string{"example.Pass"},
			pats:  []string{"example.*", "foo.*"},
			want:  []string{"foo.*"},
		},
		{
			tests: []string{"example.Pass"},
			pats:  []string{"example.*", "foo.*", "bar.*"},
			want:  []string{"bar.*", "foo.*"},
		},
		{
			tests: []string{},
			pats:  []string{"example.*", "foo.*", "bar.*"},
			want:  []string{"bar.*", "example.*", "foo.*"},
		},
		{
			tests: []string{"example.Pass", "foo.bar"},
			pats:  []string{"example.*", "foo.bar"},
			want:  []string(nil),
		},
		{
			tests: []string{"example.Pass"},
			pats:  []string{"example.*", "foo.bar"},
			want:  []string{"foo.bar"},
		},
		{
			tests: []string{"foo.bar"},
			pats:  []string{"example.*", "foo.bar"},
			want:  []string{"example.*"},
		},
		{
			tests: []string{},
			pats:  []string{"(\"name:NotExist.Pass\")"},
			want:  []string(nil),
		},
	} {
		m, err := testing.NewMatcher(tc.pats)
		if err != nil {
			t.Fatalf("Failed to compile %q: %v", tc.pats, err)
		}
		unmatched := m.UnmatchedPatterns(tc.tests)
		if diff := cmp.Diff(unmatched, tc.want); diff != "" {
			t.Errorf("UnmatchedPatterns returned unexpected patterns with tests %s, patterns %s (-got +want):\n%s", tc.tests, tc.pats, diff)
		}
	}
}
