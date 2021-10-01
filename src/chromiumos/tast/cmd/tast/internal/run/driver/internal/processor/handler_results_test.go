// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/driver/internal/processor"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

func TestResultsHandler(t *testing.T) {
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

	proc := processor.New(resDir, logging.NewMultiLogger(), nopPull)
	runProcessor(context.Background(), proc, events, nil)

	got := proc.Results()
	want := []*resultsjson.Result{
		{
			Test:   resultsjson.Test{Name: "pkg.Test1", Desc: "This is test 1"},
			Start:  epoch,
			End:    epoch,
			OutDir: filepath.Join(resDir, "tests", "pkg.Test1"),
			Errors: []resultsjson.Error{
				{Time: epoch, Reason: "Failed", File: "file.go", Line: 123, Stack: "stacktrace"},
			},
		},
		{
			Test:   resultsjson.Test{Name: "pkg.Test2", Desc: "This is test 2"},
			Start:  epoch,
			End:    epoch,
			OutDir: filepath.Join(resDir, "tests", "pkg.Test2"),
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("Results mismatch (-got +want):\n%s", diff)
	}
}
