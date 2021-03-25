// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"bytes"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

type outputSink struct {
	buf bytes.Buffer
	mw  *control.MessageWriter
}

func newOutputSink() *outputSink {
	s := outputSink{}
	s.mw = control.NewMessageWriter(&s.buf)
	return &s
}

func (s *outputSink) RunLog(msg string) error {
	return s.mw.WriteMessage(&control.RunLog{Text: msg})
}

func (s *outputSink) EntityStart(ei *protocol.Entity, outDir string) error {
	return s.mw.WriteMessage(&control.EntityStart{Info: *jsonprotocol.MustEntityInfoFromProto(ei), OutDir: outDir})
}

func (s *outputSink) EntityLog(ei *protocol.Entity, msg string) error {
	return s.mw.WriteMessage(&control.EntityLog{Text: msg, Name: ei.Name})
}

func (s *outputSink) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	// Clear Error fields except for Reason.
	je := &jsonprotocol.Error{Reason: e.Reason}
	return s.mw.WriteMessage(&control.EntityError{Error: *je, Name: ei.Name})
}

func (s *outputSink) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	// Drop timingLog.
	return s.mw.WriteMessage(&control.EntityEnd{Name: ei.Name, SkipReasons: skipReasons})
}

// ReadAll reads all control messages written to the sink.
func (s *outputSink) ReadAll() ([]control.Msg, error) {
	var msgs []control.Msg
	mr := control.NewMessageReader(&s.buf)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func TestTestOutputStream(t *gotesting.T) {
	sink := newOutputSink()
	test := &protocol.Entity{Name: "pkg.Test"}
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Log("hello")
	tout.Error(&protocol.Error{Reason: "faulty", Location: &protocol.ErrorLocation{File: "world.go"}})
	tout.Log("world")
	tout.End(nil, nil)

	got, err := sink.ReadAll()
	if err != nil {
		t.Fatal("ReadAll: ", err)
	}

	want := []control.Msg{
		&control.EntityStart{Info: *jsonprotocol.MustEntityInfoFromProto(test), OutDir: "/tmp/out"},
		&control.EntityLog{Name: "pkg.Test", Text: "hello"},
		&control.EntityError{Name: "pkg.Test", Error: jsonprotocol.Error{Reason: "faulty"}},
		&control.EntityLog{Name: "pkg.Test", Text: "world"},
		&control.EntityEnd{Name: "pkg.Test"},
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
	tout.End(nil, nil)

	got, err := sink.ReadAll()
	if err != nil {
		t.Fatal("ReadAll: ", err)
	}

	var want []control.Msg
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
	tout.End(nil, nil)
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
