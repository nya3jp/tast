// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package timing is used to collect and write timing information about a process.
package timing // import "chromiumos/tast/cmd/timing"

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type key int // unexported context.Context key type to avoid collisions with other packages

const (
	logKey key = iota // key used for attaching a Log to a context.Context
)

type timeFunc func() time.Time

// Log contains nested timing information.
type Log struct {
	stages  []*Stage
	fakeNow timeFunc // if unset, time.Now is called
}

// NewContext returns a new context that carries value l.
func NewContext(ctx context.Context, l *Log) context.Context {
	return context.WithValue(ctx, logKey, l)
}

// FromContext returns the Log value stored in ctx, if any.
func FromContext(ctx context.Context) (*Log, bool) {
	l, ok := ctx.Value(logKey).(*Log)
	return l, ok
}

// Write writes timing information to w as JSON, consisting of an array
// of stages, each represented by an array consisting of the stage's duration, name,
// and an optional array of child stages.
//
// The JSON is formatted in an attempt to increase human readability, e.g.
//
// 	[[4.000, "stage0", [
// 	         [3.000, "stage1", [
// 	                 [1.000, "stage2"],
// 	                 [2.000, "stage3"]]],
// 	         [1.000, "stage4"]]],
// 	 [0.531, "stage5"]]
func (l *Log) Write(w io.Writer) error {
	// Use a bufio.Writer to avoid any further writes after an error is encountered.
	bw := bufio.NewWriter(w)

	io.WriteString(bw, "[")
	for i, s := range l.stages {
		// The first top-level stage is on the same line as the opening '['.
		// Indent its children and all subsequent stages by a single space so they line up.
		var indent string
		if i > 0 {
			indent = " "
		}
		if err := s.write(bw, indent, " ", i == len(l.stages)-1); err != nil {
			return err
		}
	}

	io.WriteString(bw, "]\n")
	return bw.Flush() // returns first error encountered during earlier writes
}

// Start creates and returns a new named timing stage as a child of the currently-active stage.
// Stage.End should be called when the stage is completed.
func (l *Log) Start(name string) *Stage {
	var now timeFunc
	if l.fakeNow != nil {
		now = l.fakeNow
	} else {
		now = time.Now
	}

	s := &Stage{
		name:  name,
		start: now(),
		now:   now,
	}

	if l.stages == nil {
		l.stages = []*Stage{s}
	} else {
		last := l.stages[len(l.stages)-1]
		if !last.end.IsZero() {
			l.stages = append(l.stages, s)
		} else {
			p := last
			for p.children != nil && p.children[len(p.children)-1].end.IsZero() {
				p = p.children[len(p.children)-1]
			}
			if p.children == nil {
				p.children = make([]*Stage, 0)
			}
			p.children = append(p.children, s)
		}
	}

	return s
}

// Stage represents a discrete unit of work that is being timed.
type Stage struct {
	name       string
	start, end time.Time
	children   []*Stage
	now        timeFunc
}

// Elapsed returns the amount of time that passed between the start and end of the stage.
// If the stage hasn't been completed, it returns the time since the start of the stage.
func (s *Stage) Elapsed() time.Duration {
	if s.end.IsZero() {
		return s.now().Sub(s.start)
	}
	return s.end.Sub(s.start)
}

// End ends the stage.
func (s *Stage) End() {
	s.end = s.now()
}

// write writes information about the stage and its children to w as a JSON array.
// The first line of output is indented by initialIndent, while any subsequent lines (e.g.
// for child stages) are indented by followIndent. last should be true if this is the last
// entry in its parent array; otherwise a trailing comma and newline are appended.
// The caller is responsible for checking w for errors encountered while writing.
func (s *Stage) write(w *bufio.Writer, initialIndent, followIndent string, last bool) error {
	// Start the stage's array.
	mn, err := json.Marshal(&s.name)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s[%0.3f, %s", initialIndent, s.Elapsed().Seconds(), mn)

	// Print children in a nested array.
	if len(s.children) > 0 {
		io.WriteString(w, ", [\n")
		ci := followIndent + strings.Repeat(" ", 8)
		for i, c := range s.children {
			if err := c.write(w, ci, ci, i == len(s.children)-1); err != nil {
				return err
			}
		}
		io.WriteString(w, "]")
	}

	// End the stage's array.
	io.WriteString(w, "]")
	if !last {
		io.WriteString(w, ",\n")
	}
	return nil
}
