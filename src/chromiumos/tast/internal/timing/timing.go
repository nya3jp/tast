// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package timing is used to collect and write timing information about a process.
package timing

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// now is the function to return the current time. This is altered in unit tests.
var now = time.Now

// Log contains nested timing information.
type Log struct {
	// Root is a special root stage containing all stages as its descendants.
	// Its End should not be called, and its timestamps should be ignored.
	Root *Stage
}

// NewLog returns a new Log.
func NewLog() *Log {
	return &Log{Root: &Stage{}}
}

// StartTop starts and returns a new top-level stage named name.
func (l *Log) StartTop(name string) *Stage {
	return l.Root.StartChild(name)
}

// Empty returns true if l doesn't contain any stages.
func (l *Log) Empty() bool {
	l.Root.mu.Lock()
	defer l.Root.mu.Unlock()

	return len(l.Root.Children) == 0
}

// WritePretty writes timing information to w as JSON, consisting of an array
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
func (l *Log) WritePretty(w io.Writer) error {
	l.Root.mu.Lock()
	defer l.Root.mu.Unlock()

	// Use a bufio.Writer to avoid any further writes after an error is encountered.
	bw := bufio.NewWriter(w)

	io.WriteString(bw, "[")
	for i, s := range l.Root.Children {
		// The first top-level stage is on the same line as the opening '['.
		// Indent its children and all subsequent stages by a single space so they line up.
		var indent string
		if i > 0 {
			indent = " "
		}
		if err := s.writePretty(bw, indent, " ", i == len(l.Root.Children)-1); err != nil {
			return err
		}
	}

	io.WriteString(bw, "]\n")
	return bw.Flush() // returns first error encountered during earlier writes
}

// Proto returns a protobuf presentation of Log.
func (l *Log) Proto() (*protocol.TimingLog, error) {
	r, err := l.Root.Proto()
	if err != nil {
		return nil, err
	}
	return &protocol.TimingLog{Root: r}, nil
}

// jsonLog represents the JSON schema of Log.
type jsonLog struct {
	Stages []*Stage `json:"stages"`
}

// MarshalJSON marshals Log as JSON.
func (l *Log) MarshalJSON() ([]byte, error) {
	return json.Marshal(&jsonLog{Stages: l.Root.Children})
}

// UnmarshalJSON unmarshals Log as JSON.
func (l *Log) UnmarshalJSON(b []byte) error {
	var jl jsonLog
	if err := json.Unmarshal(b, &jl); err != nil {
		return err
	}
	l.Root = &Stage{Children: jl.Stages}
	return nil
}

var _ json.Marshaler = (*Log)(nil)
var _ json.Unmarshaler = (*Log)(nil)

// Stage represents a discrete unit of work that is being timed.
type Stage struct {
	Name      string    `json:"name"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Children  []*Stage  `json:"children"`

	mu sync.Mutex // protects EndTime and Children
}

// Import imports the stages from o into s, with o's top-level stages inserted as children of s.
// An error is returned if s is already ended.
func (s *Stage) Import(o *Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.EndTime.IsZero() {
		return errors.New("stage has ended")
	}

	s.Children = append(s.Children, o.Root.Children...)
	return nil
}

// StartChild creates and returns a new named timing stage as a child of s.
// Stage.End should be called when the stage is completed.
func (s *Stage) StartChild(name string) *Stage {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.EndTime.IsZero() {
		return nil
	}

	c := &Stage{
		Name:      name,
		StartTime: now(),
	}
	s.Children = append(s.Children, c)
	return c
}

// End ends the stage. Child stages are recursively examined and also ended
// (although we expect them to have already been ended).
func (s *Stage) End() {
	// Handle nil receivers returned by the package-level Start function.
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.EndTime.IsZero() {
		return
	}

	for _, c := range s.Children {
		c.End()
	}
	s.EndTime = now()
}

// writePretty writes information about the stage and its children to w as a JSON array.
// The first line of output is indented by initialIndent, while any subsequent lines (e.g.
// for child stages) are indented by followIndent. last should be true if this is the last
// entry in its parent array; otherwise a trailing comma and newline are appended.
// The caller is responsible for checking w for errors encountered while writing.
func (s *Stage) writePretty(w *bufio.Writer, initialIndent, followIndent string, last bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Start the stage's array.
	mn, err := json.Marshal(&s.Name)
	if err != nil {
		return err
	}

	var elapsed time.Duration
	if s.EndTime.IsZero() {
		elapsed = now().Sub(s.StartTime)
	} else {
		elapsed = s.EndTime.Sub(s.StartTime)
	}
	fmt.Fprintf(w, "%s[%0.3f, %s", initialIndent, elapsed.Seconds(), mn)

	// Print children in a nested array.
	if len(s.Children) > 0 {
		io.WriteString(w, ", [\n")
		ci := followIndent + strings.Repeat(" ", 8)
		for i, c := range s.Children {
			if err := c.writePretty(w, ci, ci, i == len(s.Children)-1); err != nil {
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

// Proto returns a protobuf presentation of Stage.
func (s *Stage) Proto() (*protocol.TimingStage, error) {
	start, err := timestampProto(s.StartTime)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert StartTime to protobuf")
	}
	end, err := timestampProto(s.EndTime)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert EndTime to protobuf")
	}
	var children []*protocol.TimingStage
	for _, c := range s.Children {
		child, err := c.Proto()
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return &protocol.TimingStage{
		Name:      s.Name,
		StartTime: start,
		EndTime:   end,
		Children:  children,
	}, nil
}

func timestampProto(t time.Time) (*timestamp.Timestamp, error) {
	if t.IsZero() {
		return nil, nil
	}
	return ptypes.TimestampProto(t)
}

// LogFromProto constructs Log from its protocol buffer presentation.
func LogFromProto(p *protocol.TimingLog) (*Log, error) {
	s, err := StageFromProto(p.GetRoot())
	if err != nil {
		return nil, err
	}
	return &Log{Root: s}, nil
}

// StageFromProto constructs Stage from its protocol buffer presentation.
func StageFromProto(p *protocol.TimingStage) (*Stage, error) {
	var start, end time.Time
	if ts := p.GetStartTime(); ts != nil {
		var err error
		start, err = ptypes.Timestamp(ts)
		if err != nil {
			return nil, err
		}
	}
	if ts := p.GetEndTime(); ts != nil {
		var err error
		end, err = ptypes.Timestamp(ts)
		if err != nil {
			return nil, err
		}
	}
	var children []*Stage
	for _, c := range p.GetChildren() {
		child, err := StageFromProto(c)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return &Stage{
		Name:      p.GetName(),
		StartTime: start,
		EndTime:   end,
		Children:  children,
	}, nil
}
