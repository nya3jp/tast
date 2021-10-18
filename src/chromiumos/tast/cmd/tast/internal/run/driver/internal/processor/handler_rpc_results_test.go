// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/cmd/tast/internal/run/driver/internal/processor"
	"chromiumos/tast/cmd/tast/internal/run/fakereports"
	"chromiumos/tast/cmd/tast/internal/run/reporting"
	frameworkprotocol "chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
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

	proc := processor.New(resDir, logging.NewMultiLogger(), nopDiagnose, nopPull, nil, client)
	runProcessor(context.Background(), proc, events, nil)

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
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time")); diff != "" {
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

	proc := processor.New(resDir, logging.NewMultiLogger(), nopDiagnose, nopPull, nil, client)
	runProcessor(context.Background(), proc, events, nil)

	got := srv.Results()
	want := []*frameworkprotocol.ReportResultRequest{
		{Test: "pkg.Test1", Errors: []*frameworkprotocol.ErrorReport{{Reason: "Failed"}, {Reason: "Failed"}}},
		{Test: "pkg.Test2", Errors: []*frameworkprotocol.ErrorReport{{Reason: "Failed"}, {Reason: "Failed"}}},
		// Third test is not executed.
	}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time")); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}
}