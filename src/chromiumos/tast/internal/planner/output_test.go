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
	"chromiumos/tast/timing"
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

func (s *outputSink) TestStart(t *testing.TestInfo) error {
	return s.mw.WriteMessage(&control.TestStart{Test: *t})
}

func (s *outputSink) TestLog(t *testing.TestInfo, msg string) error {
	return s.mw.WriteMessage(&control.TestLog{Text: msg})
}

func (s *outputSink) TestError(t *testing.TestInfo, e *testing.Error) error {
	// Clear Error fields except for Reason.
	e = &testing.Error{Reason: e.Reason}
	return s.mw.WriteMessage(&control.TestError{Error: *e})
}

func (s *outputSink) TestEnd(t *testing.TestInfo, skipReasons []string, timingLog *timing.Log) error {
	// Drop timingLog.
	return s.mw.WriteMessage(&control.TestEnd{Name: t.Name, SkipReasons: skipReasons})
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
	test := &testing.TestInfo{Name: "pkg.Test"}
	tout := newTestOutputStream(sink, test)

	tout.Start()
	tout.Log("hello")
	tout.Error(&testing.Error{Reason: "faulty", File: "world.go"})
	tout.Log("world")
	tout.End(nil, nil)

	got, err := sink.ReadAll()
	if err != nil {
		t.Fatal("ReadAll: ", err)
	}

	want := []control.Msg{
		&control.TestStart{Test: *test},
		&control.TestLog{Text: "hello"},
		&control.TestError{Error: testing.Error{Reason: "faulty"}},
		&control.TestLog{Text: "world"},
		&control.TestEnd{Name: "pkg.Test"},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}
