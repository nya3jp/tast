// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	frameworkprotocol "chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/processor"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/fakereports"
	"chromiumos/tast/internal/run/reporting"
)

func TestRPCResultsHandler_Results(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		// Fixture starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		// First test runs with 1 error.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test1", Description: "This is test 1"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test1", Error: &protocol.Error{Reason: "Failed", Location: &protocol.ErrorLocation{File: "file.go", Line: 123, Stack: "stacktrace"}}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test1"},
		// Fixture reports an error.
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "fixture", Error: &protocol.Error{Reason: "Failed", Location: &protocol.ErrorLocation{File: "fixture.go", Line: 456, Stack: "stacktrace"}}},
		// Second test runs with no error.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test2", Description: "This is test 2"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test2"},
		// Fixture ends.
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
	}

	srv, stopFunc, addr := fakereports.Start(t, 0)
	defer stopFunc()

	client, err := reporting.NewRPCClient(context.Background(), addr)
	if err != nil {
		t.Fatalf("Failed to connect to RPC results server: %v", err)
	}
	defer client.Close()

	hs := newHandlers(resDir, logging.NewMultiLogger(), nopPull, nil, client)
	proc := processor.New(resDir, nopDiagnose, hs)
	runProcessor(context.Background(), proc, events, nil)

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	got := srv.Results()
	want := []*frameworkprotocol.ReportResultRequest{
		{
			Test: "pkg.Test1",
			Errors: []*frameworkprotocol.ErrorReport{{
				Reason: "Failed",
				File:   "file.go",
				Line:   123,
				Stack:  "stacktrace",
			}},
		},
		{
			Test: "pkg.Test2",
		},
	}
	// make sure StartTime and Duration are not nil.
	for _, r := range got {
		if r.StartTime == nil || r.Duration == nil {
			t.Errorf("Test result for %s return nil on start time or duration", r.Test)
		}
	}
	cmpOptIgnoreErrFields := cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time")
	cmpOptIgnoreTimeFields := cmpopts.IgnoreFields(frameworkprotocol.ReportResultRequest{}, "StartTime", "Duration")
	cmpOptIgnoreUnexported := cmpopts.IgnoreUnexported(frameworkprotocol.ReportResultRequest{})
	cmpOptIgnoreUnexportedErr := cmpopts.IgnoreUnexported(frameworkprotocol.ErrorReport{})
	if diff := cmp.Diff(got, want,
		cmpOptIgnoreTimeFields, cmpOptIgnoreErrFields,
		cmpOptIgnoreUnexported, cmpOptIgnoreUnexportedErr); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}
}

func TestRPCResultsHandler_Terminate(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		// Fixture starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		// First test runs with 2 errors.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test1"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test1", Error: &protocol.Error{Reason: "Failed"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test1", Error: &protocol.Error{Reason: "Failed"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test1"},
		// Fixture reports 2 errors.
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "fixture", Error: &protocol.Error{Reason: "Failed"}},
		// Second test runs with 2 errors.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test2"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test2", Error: &protocol.Error{Reason: "Failed"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test2", Error: &protocol.Error{Reason: "Failed"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test2"},
		// Third test runs with no error.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test3"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test3"},
		// Fixture ends.
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
	}

	srv, stopFunc, addr := fakereports.Start(t, 2)
	defer stopFunc()

	client, err := reporting.NewRPCClient(context.Background(), addr)
	if err != nil {
		t.Fatalf("Failed to connect to RPC results server: %v", err)
	}
	defer client.Close()

	hs := newHandlers(resDir, logging.NewMultiLogger(), nopPull, nil, client)
	proc := processor.New(resDir, nopDiagnose, hs)
	runProcessor(context.Background(), proc, events, nil)

	if err := proc.FatalError(); err == nil {
		t.Error("Processor did not see fatal errors unexpectedly")
	}

	got := srv.Results()
	want := []*frameworkprotocol.ReportResultRequest{
		{Test: "pkg.Test1", Errors: []*frameworkprotocol.ErrorReport{{Reason: "Failed"}, {Reason: "Failed"}}},
		{Test: "pkg.Test2", Errors: []*frameworkprotocol.ErrorReport{{Reason: "Failed"}, {Reason: "Failed"}}},
		// Third test is not executed.
	}
	cmpOptIgnoreErrFields := cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time")
	cmpOptIgnoreTimeFields := cmpopts.IgnoreFields(frameworkprotocol.ReportResultRequest{}, "StartTime", "Duration")
	cmpOptIgnoreUnexported := cmpopts.IgnoreUnexported(frameworkprotocol.ReportResultRequest{})
	cmpOptIgnoreUnexportedErr := cmpopts.IgnoreUnexported(frameworkprotocol.ErrorReport{})
	if diff := cmp.Diff(got, want,
		cmpOptIgnoreTimeFields, cmpOptIgnoreErrFields,
		cmpOptIgnoreUnexported, cmpOptIgnoreUnexportedErr); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}
}
