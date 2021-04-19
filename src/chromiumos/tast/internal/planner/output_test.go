// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

type outputSink struct {
	msgs []protocol.Event
}

func newOutputSink() *outputSink {
	return &outputSink{}
}

func (s *outputSink) RunLog(msg string) error {
	s.msgs = append(s.msgs, &protocol.RunLogEvent{
		Text: msg,
	})
	return nil
}

func (s *outputSink) EntityStart(ei *protocol.Entity, outDir string) error {
	s.msgs = append(s.msgs, &protocol.EntityStartEvent{
		Entity: ei,
		OutDir: outDir,
	})
	return nil
}

func (s *outputSink) EntityLog(ei *protocol.Entity, msg string) error {
	s.msgs = append(s.msgs, &protocol.EntityLogEvent{
		EntityName: ei.GetName(),
		Text:       msg,
	})
	return nil
}

func (s *outputSink) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	s.msgs = append(s.msgs, &protocol.EntityErrorEvent{
		EntityName: ei.GetName(),
		// Clear Error fields except for Reason.
		Error: &protocol.Error{Reason: e.GetReason()},
	})
	return nil
}

func (s *outputSink) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	// Drop timingLog.
	var skip *protocol.Skip
	if len(skipReasons) > 0 {
		skip = &protocol.Skip{Reasons: skipReasons}
	}
	s.msgs = append(s.msgs, &protocol.EntityEndEvent{EntityName: ei.GetName(), Skip: skip})
	return nil
}

// ReadAll reads all control messages written to the sink.
func (s *outputSink) ReadAll() []protocol.Event {
	return s.msgs
}

func TestTestOutputStream(t *gotesting.T) {
	sink := newOutputSink()
	test := &protocol.Entity{Name: "pkg.Test"}
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Log("hello")
	tout.Error(&protocol.Error{Reason: "faulty", Location: &protocol.ErrorLocation{File: "world.go"}})
	tout.Log("world")
	tout.End(nil, timing.NewLog())

	got := sink.ReadAll()

	want := []protocol.Event{
		&protocol.EntityStartEvent{Entity: test, OutDir: "/tmp/out"},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "hello"},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "faulty"}},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "world"},
		&protocol.EntityEndEvent{EntityName: "pkg.Test"},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestTestOutputStreamUnnamedEntity(t *gotesting.T) {
	sink := newOutputSink()
	test := &protocol.Entity{} // unnamed entity
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Log("hello")
	tout.Error(&protocol.Error{Reason: "faulty", Location: &protocol.ErrorLocation{File: "world.go"}})
	tout.Log("world")
	tout.End(nil, timing.NewLog())

	got := sink.ReadAll()

	var want []protocol.Event
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestTestOutputStreamErrors(t *gotesting.T) {
	sink := newOutputSink()
	test := &protocol.Entity{Name: "pkg.Test"}
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Error(&protocol.Error{Reason: "error1", Location: &protocol.ErrorLocation{File: "test1.go"}})
	tout.Error(&protocol.Error{Reason: "error2", Location: &protocol.ErrorLocation{File: "test2.go"}})
	tout.End(nil, timing.NewLog())
	tout.Error(&protocol.Error{Reason: "error3", Location: &protocol.ErrorLocation{File: "test3.go"}}) // error after End is ignored

	got := tout.Errors()
	want := []*protocol.Error{
		{Reason: "error1", Location: &protocol.ErrorLocation{File: "test1.go"}},
		{Reason: "error2", Location: &protocol.ErrorLocation{File: "test2.go"}},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Errors mismatch (-got +want):\n", diff)
	}
}
