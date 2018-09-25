// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package stack

import (
	"regexp"
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	// Stack depth here should be shorter than maxDepth.
	trace := New(0).String()

	lines := strings.Split(trace, "\n")
	if len(lines) <= 2 {
		t.Fatalf("Stack trace is too short: %q", trace)
	}

	firstRegexp := regexp.MustCompile(`^\tat chromiumos/tast/errors/stack\.TestShort \(stack_test.go:\d+\)$`)
	if s := lines[0]; !firstRegexp.MatchString(s) {
		t.Errorf("First line of stack trace is wrong: expected to match %q, got %q", firstRegexp, s)
	}

	if s := lines[len(lines)-1]; s == ellipsis {
		t.Errorf("Stack trace ends with ellipsis")
	}
}

func getDeepStack(depth int) Stack {
	if depth == 0 {
		return New(0)
	}
	return getDeepStack(depth - 1)
}

func TestLong(t *testing.T) {
	trace := getDeepStack(maxDepth).String()

	lines := strings.Split(trace, "\n")
	if len(lines) != maxDepth+1 {
		t.Fatalf("Stack trace has wrong number of lines: expected %d, got %d", maxDepth+1, len(lines))
	}

	re := regexp.MustCompile(`^\tat chromiumos/tast/errors/stack\.getDeepStack \(stack_test.go:\d+\)$`)
	for i, line := range lines {
		if i < len(lines)-1 {
			if !re.MatchString(line) {
				t.Errorf("Line %d of stack trace is wrong: expected to match %q, got %q", i, re, line)
			}
		} else {
			if line != ellipsis {
				t.Errorf("Stack trace does not end with ellipsis")
			}
		}
	}
}
