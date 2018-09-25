// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package stack provides a utility to capture and format a stack trace.
// This is not intended to be used directly by Tast tests; use the errors
// package instead.
package stack

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	maxDepth = 8 // maximum number of stack frames to record

	ellipsis = "\t..." // trailing marker line added if stack trace is too long
)

// Stack holds a snapshot of program counters.
type Stack []uintptr

// New captures a stack trace. skip specifies the number of frames to skip from
// a stack trace. skip=0 records stack.New call as the innermost frame.
func New(skip int) Stack {
	pc := make([]uintptr, maxDepth+1)
	pc = pc[:runtime.Callers(skip+2, pc)]
	return Stack(pc)
}

// String formats a stack trace to a human-friendly text.
func (s Stack) String() string {
	var lines []string

	// Use runtime.CallerFrames to parse results of runtime.Callers correctly.
	// https://github.com/golang/go/issues/19426
	// https://talks.godoc.org/github.com/davecheney/go-1.9-release-party/presentation.slide#20
	cf := runtime.CallersFrames(s)
	for {
		f, more := cf.Next()
		line := fmt.Sprintf("\tat %s (%s:%d)", f.Function, filepath.Base(f.File), f.Line)
		lines = append(lines, line)
		if !more {
			break
		} else if len(lines) >= maxDepth {
			lines = append(lines, ellipsis)
			break
		}
	}
	return strings.Join(lines, "\n")
}
