// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package timing is used to collect and write timing information about a process.
package timing

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	Stages  []*Stage `json:"stages"`
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

// Start starts and returns a new Stage named name within the Log attached
// to ctx. If no Log is attached to ctx, nil is returned. It is safe to call Close
// on a nil stage.
//
// Example usage to report the time used until the end of the current function:
//
//	defer timing.Start(ctx, "my_stage").End()
func Start(ctx context.Context, name string) *Stage {
	l, ok := FromContext(ctx)
	if !ok {
		return nil
	}
	return l.Start(name)
}

// Empty returns true if l doesn't contain any stages.
func (l *Log) Empty() bool {
	return len(l.Stages) == 0
}

// Write writes timing information to w as JSON, consisting of an array
// of stages, each represented by an array consisting of the stage's duration, name,
// and an optional array of child stages.
//
// Note that this format is lossy and differs from that used by json.Marshaler.
//
// Output is intended to improve human readability:
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
	for i, s := range l.Stages {
		// The first top-level stage is on the same line as the opening '['.
		// Indent its children and all subsequent stages by a single space so they line up.
		var indent string
		if i > 0 {
			indent = " "
		}
		if err := s.write(bw, indent, " ", i == len(l.Stages)-1); err != nil {
			return err
		}
	}

	io.WriteString(bw, "]\n")
	return bw.Flush() // returns first error encountered during earlier writes
}

// Import the stages from o into l, with o's top-level stages inserted as children of the currently-active stage in l.
// An error is returned if l does not contain an active stage.
func (l *Log) Import(o *Log) error {
	if l.Stages == nil || !l.Stages[len(l.Stages)-1].active() {
		return errors.New("no currently-active stage")
	}
	l.add(o.Stages...)
	return nil
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
		Name:      name,
		StartTime: now(),
		now:       now,
	}
	l.add(s)
	return s
}

// add adds stages as children of the currently-active stage, if any.
// If no stage is currently active, the stages are added as top-level stages.
func (l *Log) add(stages ...*Stage) {
	if len(stages) == 0 {
		return
	}

	// If there are no stages or the last one isn't active, append the new stages.
	if l.Stages == nil || !l.Stages[len(l.Stages)-1].active() {
		l.Stages = append(l.Stages, stages...)
		return
	}

	// Otherwise, find the currently-active stage and append the new stages as children.
	p := l.Stages[len(l.Stages)-1]
	for p.Children != nil && p.Children[len(p.Children)-1].active() {
		p = p.Children[len(p.Children)-1]
	}
	if p.Children == nil {
		p.Children = make([]*Stage, 0)
	}
	p.Children = append(p.Children, stages...)
}

// Stage represents a discrete unit of work that is being timed.
type Stage struct {
	Name      string    `json:"name"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Children  []*Stage  `json:"children"`
	now       timeFunc
}

// Elapsed returns the amount of time that passed between the start and end of the stage.
// If the stage hasn't been completed, it returns the time since the start of the stage.
func (s *Stage) Elapsed() time.Duration {
	if s.active() {
		return s.now().Sub(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// End ends the stage. Child stages are recursively examined and also ended
// (although we expect them to have already been ended).
func (s *Stage) End() {
	// Handle nil receivers returned by the package-level Start function.
	if s == nil {
		return
	}

	for _, c := range s.Children {
		c.End()
	}
	if s.active() {
		s.EndTime = s.now()
	}
}

// active returns true if s is still active (i.e. not ended). Child stages are not checked.
func (s *Stage) active() bool {
	return s.EndTime.IsZero()
}

// write writes information about the stage and its children to w as a JSON array.
// The first line of output is indented by initialIndent, while any subsequent lines (e.g.
// for child stages) are indented by followIndent. last should be true if this is the last
// entry in its parent array; otherwise a trailing comma and newline are appended.
// The caller is responsible for checking w for errors encountered while writing.
func (s *Stage) write(w *bufio.Writer, initialIndent, followIndent string, last bool) error {
	// Start the stage's array.
	mn, err := json.Marshal(&s.Name)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s[%0.3f, %s", initialIndent, s.Elapsed().Seconds(), mn)

	// Print children in a nested array.
	if len(s.Children) > 0 {
		io.WriteString(w, ", [\n")
		ci := followIndent + strings.Repeat(" ", 8)
		for i, c := range s.Children {
			if err := c.write(w, ci, ci, i == len(s.Children)-1); err != nil {
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
