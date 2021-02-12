// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"bytes"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/testing"
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

func (s *outputSink) EntityStart(ei *testing.EntityInfo, outDir string) error {
	return s.mw.WriteMessage(&control.EntityStart{Info: *ei, OutDir: outDir})
}

func (s *outputSink) EntityLog(ei *testing.EntityInfo, msg string) error {
	return s.mw.WriteMessage(&control.EntityLog{Text: msg, Name: ei.Name})
}

func (s *outputSink) EntityError(ei *testing.EntityInfo, e *testing.Error) error {
	// Clear Error fields except for Reason.
	e = &testing.Error{Reason: e.Reason}
	return s.mw.WriteMessage(&control.EntityError{Error: *e, Name: ei.Name})
}

func (s *outputSink) EntityEnd(ei *testing.EntityInfo, skipReasons []string, timingLog *timing.Log) error {
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
	test := &testing.EntityInfo{Name: "pkg.Test"}
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Log("hello")
	tout.Error(&testing.Error{Reason: "faulty", File: "world.go"})
	tout.Log("world")
	tout.End(nil, nil)

	got, err := sink.ReadAll()
	if err != nil {
		t.Fatal("ReadAll: ", err)
	}

	want := []control.Msg{
		&control.EntityStart{Info: *test, OutDir: "/tmp/out"},
		&control.EntityLog{Name: "pkg.Test", Text: "hello"},
		&control.EntityError{Name: "pkg.Test", Error: testing.Error{Reason: "faulty"}},
		&control.EntityLog{Name: "pkg.Test", Text: "world"},
		&control.EntityEnd{Name: "pkg.Test"},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestTestOutputStreamUnnamedEntity(t *gotesting.T) {
	sink := newOutputSink()
	test := &testing.EntityInfo{} // unnamed entity
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Log("hello")
	tout.Error(&testing.Error{Reason: "faulty", File: "world.go"})
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
	test := &testing.EntityInfo{Name: "pkg.Test"}
	tout := newEntityOutputStream(sink, test)

	tout.Start("/tmp/out")
	tout.Error(&testing.Error{Reason: "error1", File: "test1.go"})
	tout.Error(&testing.Error{Reason: "error2", File: "test2.go"})
	tout.End(nil, nil)
	tout.Error(&testing.Error{Reason: "error3", File: "test3.go"}) // error after End is ignored

	got := tout.Errors()
	want := []*testing.Error{
		{Reason: "error1", File: "test1.go"},
		{Reason: "error2", File: "test2.go"},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Errors mismatch (-got +want):\n", diff)
	}
}
