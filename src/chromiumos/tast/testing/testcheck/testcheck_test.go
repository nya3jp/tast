// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcheck_test

import (
	gotesting "testing"
	"time"

	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testing/testcheck"
)

func TestMain(m *gotesting.M) {
	restore := testcheck.SetAllTestsforTest([]*testing.TestInstance{{
		Name: "foo.FirstTest", Timeout: 10 * time.Minute, Attr: []string{"A", "B"}, SoftwareDeps: []string{"A", "B"},
	}, {
		Name: "foo.SecondTest", Timeout: 5 * time.Minute, Attr: []string{"A", "C"}, SoftwareDeps: []string{"A", "C"},
	}})
	defer restore()
	m.Run()
}

func TestGlob(t *gotesting.T) {
	for _, tc := range []struct {
		name string
		glob string
		want bool
	}{
		{"foo.Test", "*", true},
		{"foo.Test", "boo.*", false},
		{"foo.Test", "foo.*", true},
	} {
		if got := testcheck.Glob(t, tc.glob)(&testing.TestInstance{Name: tc.name}); got != tc.want {
			t.Errorf("Glob(%q)(%q) = %v, want %v", tc.glob, tc.name, got, tc.want)
		}
	}
}

func TestTimeout(t *gotesting.T) {
	for _, tc := range []struct {
		timeout  time.Duration
		wantFail bool
	}{
		{1 * time.Minute, false},
		{7 * time.Minute, true},
	} {
		tt := &gotesting.T{}
		testcheck.Timeout(tt, testcheck.Glob(t, "*"), tc.timeout)
		if tt.Failed() != tc.wantFail {
			t.Errorf("Timeout(%v) Failed = %v, want %v", tc.timeout, tt.Failed(), tc.wantFail)
		}
	}
}

func TestAttr(t *gotesting.T) {
	for _, tc := range []struct {
		attr     []string
		wantFail bool
	}{
		{[]string{}, false},
		{[]string{"A"}, false},
		{[]string{"A", "B|C"}, false},
		{[]string{"D"}, true},
		{[]string{"A", "B|D"}, true},
	} {
		tt := &gotesting.T{}
		testcheck.Attr(tt, testcheck.Glob(t, "*"), tc.attr)
		if tt.Failed() != tc.wantFail {
			t.Errorf("Attr(%v) Failed = %v, want %v", tc.attr, tt.Failed(), tc.wantFail)
		}
	}
}

func TestIfAttr(t *gotesting.T) {
	for _, tc := range []struct {
		criteriaAttr []string
		attr         []string
		wantFail     bool
	}{
		{[]string{}, []string{}, false},
		{[]string{"A"}, []string{"B|C"}, false},
		{[]string{"A"}, []string{"B"}, true},
		{[]string{"B|C"}, []string{"A"}, false},
		{[]string{"D"}, []string{"B"}, false},
		{[]string{"A", "B|C"}, []string{"D", "E|F"}, true},
	} {
		tt := &gotesting.T{}
		testcheck.IfAttr(tt, testcheck.Glob(t, "*"), tc.criteriaAttr, tc.attr)
		if tt.Failed() != tc.wantFail {
			t.Errorf("Attr(%v) Failed = %v, want %v", tc.attr, tt.Failed(), tc.wantFail)
		}
	}
}

func TestSoftwareDeps(t *gotesting.T) {
	for _, tc := range []struct {
		deps     []string
		wantFail bool
	}{
		{[]string{}, false},
		{[]string{"A"}, false},
		{[]string{"A", "B|C"}, false},
		{[]string{"D"}, true},
		{[]string{"A", "B|D"}, true},
	} {
		tt := &gotesting.T{}
		testcheck.SoftwareDeps(tt, testcheck.Glob(t, "*"), tc.deps)
		if tt.Failed() != tc.wantFail {
			t.Errorf("SoftwareDeps(%v) Failed = %v, want %v", tc.deps, tt.Failed(), tc.wantFail)
		}
	}
}
