// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package stack

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const maxDepth = 8

type Stack struct {
	pc   []uintptr
	more bool
}

func New(skip int) *Stack {
	s := &Stack{pc: make([]uintptr, maxDepth+1)}
	s.pc = s.pc[:runtime.Callers(skip+2, s.pc)]
	if len(s.pc) > maxDepth {
		s.pc = s.pc[:maxDepth]
		s.more = true
	}
	return s
}

func (s *Stack) String() string {
	var lines []string
	for _, pc := range s.pc {
		fc := runtime.FuncForPC(pc)
		var loc string
		if fc == nil {
			loc = "???"
		} else {
			fn, ln := fc.FileLine(pc)
			loc = fmt.Sprintf("%s (%s:%d)", fc.Name(), filepath.Base(fn), ln)
		}
		lines = append(lines, fmt.Sprintf("\tat %s", loc))
	}
	if s.more {
		lines = append(lines, "\t...")
	}
	return strings.Join(lines, "\n")
}
