// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/processor"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/reporting"
	"go.chromium.org/tast/core/internal/run/resultsjson"
)

func unmarshalStreamedResults(b []byte) ([]*resultsjson.Result, error) {
	decoder := json.NewDecoder(bytes.NewBuffer(b))
	var results []*resultsjson.Result
	for decoder.More() {
		var r resultsjson.Result
		if err := decoder.Decode(&r); err != nil {
			return nil, err
		}
		results = append(results, &r)
	}
	return results, nil
}

func TestStreamedResultsHandler(t *testing.T) {
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

	hs := newHandlers(resDir, logging.NewMultiLogger(), nopPull, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	runProcessor(context.Background(), proc, events, nil)

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(resDir, reporting.StreamedResultsFilename))
	if err != nil {
		t.Fatalf("Failed to read %s: %v", reporting.StreamedResultsFilename, err)
	}

	got, _ := unmarshalStreamedResults(b)
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
