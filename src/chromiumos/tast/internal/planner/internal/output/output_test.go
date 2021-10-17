// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package output_test

import (
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/planner/internal/output/outputtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

func TestTestOutputStream(t *gotesting.T) {
	sink := outputtest.NewSink()
	test := &protocol.Entity{Name: "pkg.Test"}
	tout := output.NewEntityStream(sink, test)

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
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestTestOutputStreamUnnamedEntity(t *gotesting.T) {
	sink := outputtest.NewSink()
	test := &protocol.Entity{} // unnamed entity
	tout := output.NewEntityStream(sink, test)

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
	sink := outputtest.NewSink()
	test := &protocol.Entity{Name: "pkg.Test"}
	tout := output.NewEntityStream(sink, test)

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
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Error("Errors mismatch (-got +want):\n", diff)
	}
}
