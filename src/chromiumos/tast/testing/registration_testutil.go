// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"testing"
	"time"
)

func getTests(t *testing.T, pattern string) []*Test {
	tests, err := GlobalRegistry().TestsForPatterns([]string{pattern})
	if err != nil {
		t.Fatalf("Failed to get tests for %s: %v", pattern, err)
	}
	if len(tests) == 0 {
		t.Fatalf("No tests matched for %s", pattern)
	}
	return tests
}

// CheckTimeout checks that tests matched by pattern have timeout no less than minTimeout.
func CheckTimeout(t *testing.T, pattern string, minTimeout time.Duration) {
	for _, tst := range getTests(t, pattern) {
		if tst.Timeout < minTimeout {
			t.Errorf("%s: timeout is too short (%v < %v)", tst.Name, tst.Timeout, minTimeout)
		}
	}
}

// CheckSoftwareDeps checks that tests matched by pattern declares requiredDeps as software dependencies.
func CheckSoftwareDeps(t *testing.T, pattern string, requiredDeps []string) {
	for _, tst := range getTests(t, pattern) {
		deps := make(map[string]struct{})
		for _, d := range tst.SoftwareDeps {
			deps[d] = struct{}{}
		}
		for _, d := range requiredDeps {
			if _, ok := deps[d]; !ok {
				t.Errorf("%s: missing software dependency %q", tst.Name, d)
			}
		}
	}
}
