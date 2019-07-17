// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcheck provides common functions to check test definitions.
package testcheck

import (
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/testing"
)

func getTests(t *gotesting.T, glob string) []*testing.TestCase {
	tests, err := testing.GlobalRegistry().TestsForGlobs([]string{glob})
	if err != nil {
		t.Fatalf("Failed to get tests for %s: %v", glob, err)
	}
	if len(tests) == 0 {
		t.Fatalf("No tests matched for %s", glob)
	}
	return tests
}

// Timeout checks that tests matched by glob have timeout no less than minTimeout.
func Timeout(t *gotesting.T, glob string, minTimeout time.Duration) {
	for _, tst := range getTests(t, glob) {
		if tst.Timeout < minTimeout {
			t.Errorf("%s: timeout is too short (%v < %v)", tst.Name, tst.Timeout, minTimeout)
		}
	}
}

// SoftwareDeps checks that tests matched by glob declare requiredDeps as software dependencies.
// requiredDeps is a list of items which the test's SoftwareDeps needs to
// satisfy. Each item is one or '|'-connected multiple software feature names,
// and SoftwareDeps must contain at least one of them.
func SoftwareDeps(t *gotesting.T, glob string, requiredDeps []string) {
	for _, tst := range getTests(t, glob) {
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
