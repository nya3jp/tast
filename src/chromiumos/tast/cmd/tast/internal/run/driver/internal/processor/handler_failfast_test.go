// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/driver/internal/failfast"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/processor"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

func TestFailFastHandler(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		// Fixture starts and reports an error.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "fixture"},
		// First test passes.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test1"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test1"},
		// Second test fails with 3 errors.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test2"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test2"},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test2"},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test2"},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test2"},
		// Third test fails with 2 errors.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test3"}},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test3"},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test3"},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test3"},
		// Forth test passes.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test4"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test4"},
		// Fixture ends.
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
	}

	// Abort after 2 test failures.
	counter := failfast.NewCounter(2)

	proc := processor.New(resDir, logging.NewMultiLogger(), nopDiagnose, nopPull, counter, nil)
	runProcessor(context.Background(), proc, events, nil)

	if err := proc.FatalError(); err == nil {
		t.Error("Processor did not see fatal errors unexpectedly")
	}

	got := proc.Results()
	want := []*resultsjson.Result{
		{
			Test:   resultsjson.Test{Name: "pkg.Test1"},
			Start:  epoch,
			End:    epoch,
			OutDir: filepath.Join(resDir, "tests", "pkg.Test1"),
		},
		{
			Test:   resultsjson.Test{Name: "pkg.Test2"},
			Start:  epoch,
			End:    epoch,
			OutDir: filepath.Join(resDir, "tests", "pkg.Test2"),
			Errors: []resultsjson.Error{{Time: epoch}, {Time: epoch}, {Time: epoch}},
		},
		{
			Test:   resultsjson.Test{Name: "pkg.Test3"},
			Start:  epoch,
			End:    epoch,
			OutDir: filepath.Join(resDir, "tests", "pkg.Test3"),
			Errors: []resultsjson.Error{{Time: epoch}, {Time: epoch}},
		},
		// Forth test is not reported due to early abort.
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("Results mismatch (-got +want):\n%s", diff)
	}
}
