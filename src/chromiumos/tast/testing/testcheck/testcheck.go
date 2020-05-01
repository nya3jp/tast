// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcheck provides common functions to check test definitions.
package testcheck

import (
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/internal/testing"
)

// TestFilter defines the condition whether or not the test should be checked.
type TestFilter func(t *testing.TestInstance) bool

func getTests(t *gotesting.T, f TestFilter) []*testing.TestInstance {
	var tests []*testing.TestInstance
	for _, tst := range testing.GlobalRegistry().AllTests() {
		if f(tst) {
			tests = append(tests, tst)
		}
	}
	if len(tests) == 0 {
		t.Fatalf("No tests matched")
	}
	return tests
}

// Glob returns a TestFilter which returns true for a test if the test name
// matches with the given glob pattern.
func Glob(t *gotesting.T, glob string) TestFilter {
	re, err := testing.NewTestGlobRegexp(glob)
	if err != nil {
		t.Fatalf("Bad glob %q: %v", glob, err)
	}
	return func(t *testing.TestInstance) bool {
		return re.MatchString(t.Name)
	}
}

// Timeout checks that tests matched by f have timeout no less than minTimeout.
func Timeout(t *gotesting.T, f TestFilter, minTimeout time.Duration) {
	for _, tst := range getTests(t, f) {
		if tst.Timeout < minTimeout {
			t.Errorf("%s: timeout is too short (%v < %v)", tst.Name, tst.Timeout, minTimeout)
		}
	}
}

// SoftwareDeps checks that tests matched by f declare requiredDeps as software dependencies.
// requiredDeps is a list of items which the test's SoftwareDeps needs to
// satisfy. Each item is one or '|'-connected multiple software feature names,
// and SoftwareDeps must contain at least one of them.
func SoftwareDeps(t *gotesting.T, f TestFilter, requiredDeps []string) {
	for _, tst := range getTests(t, f) {
		deps := make(map[string]struct{})
		for _, d := range tst.SoftwareDeps {
			deps[d] = struct{}{}
		}
	CheckLoop:
		for _, d := range requiredDeps {
			for _, item := range strings.Split(d, "|") {
				if _, ok := deps[item]; ok {
					continue CheckLoop
				}
			}
			t.Errorf("%s: missing software dependency %q", tst.Name, d)
		}
	}
}
