// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/processor"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/resultsjson"
	"go.chromium.org/tast/core/internal/xcontext"
)

// TestPreprocessor_SameEntity checks preprocessor's behavior on receiving
// multiple EntityStart/EntityEnd events for the same entity.
func TestPreprocessor_SameEntity(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test"},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test"},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
	}

	hs := newHandlers(resDir, logging.NewMultiLogger(), nopPull, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	runProcessor(context.Background(), proc, events, errors.New("something went wrong"))

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	// Output directories are created with suffixes to avoid conflicts.
	for _, relPath := range []string{
		"fixtures/fixture",
		"fixtures/fixture.1",
		"tests/test",
		"tests/test.1",
	} {
		if _, err := os.Stat(filepath.Join(resDir, relPath)); err != nil {
			t.Errorf("%s was not created: %v", relPath, err)
		}
	}
}

// TestPreprocessor_MissingEntityEnd checks preprocessor's behavior for
// handling missing EntityEnd messages.
func TestPreprocessor_MissingEntityEnd(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		// Fixture starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		// First test starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test"}},
		// Test runner crashes.
	}

	logger := logging.NewMultiLogger()
	ctx := logging.AttachLogger(context.Background(), logger)

	hs := newHandlers(resDir, logger, nopPull, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	runProcessor(ctx, proc, events, errors.New("something went wrong"))

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	// loggingHandler should be notified for artificially generated
	// EntityEnd events for the two entities.
	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{"fixtures/fixture/log.txt", "something went wrong"},
		{"fixtures/fixture/log.txt", "Completed fixture fixture"},
		{"tests/test/log.txt", "something went wrong"},
		{"tests/test/log.txt", "Completed test test"},
	} {
		b, err := os.ReadFile(filepath.Join(resDir, tc.relPath))
		if err != nil {
			t.Errorf("Failed to read %s: %v", tc.relPath, err)
			continue
		}
		s := string(b)
		if !strings.Contains(s, tc.want) {
			t.Errorf("%s doesn't contain an expected string: got %q, want %q", tc.relPath, s, tc.want)
		}
	}
}

func TestPreprocessor_Diagnose(t *testing.T) {
	resDir := t.TempDir()

	fakeDiagnose := func(ctx context.Context, outDir string) string {
		wantDir := filepath.Join(resDir, "tests/test2")
		if outDir != wantDir {
			t.Errorf("fakeDiagnose: Unexpected output directory: got %q, want %q", outDir, wantDir)
		}
		return "detailed diagnosis"
	}

	events := []protocol.Event{
		// First test starts and passes.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test1"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test1"},
		// Second test starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test2"}},
		// Test runner crashes.
	}

	logger := logging.NewMultiLogger()
	ctx := logging.AttachLogger(context.Background(), logger)

	hs := newHandlers(resDir, logger, nopPull, nil, nil)
	proc := processor.New(resDir, fakeDiagnose, hs, "cros")
	runProcessor(ctx, proc, events, errors.New("something went wrong"))

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	got := proc.Results()
	want := []*resultsjson.Result{
		{
			Test:   resultsjson.Test{Name: "test1"},
			OutDir: filepath.Join(resDir, "tests", "test1"),
		},
		{
			Test:   resultsjson.Test{Name: "test2"},
			OutDir: filepath.Join(resDir, "tests", "test2"),
			Errors: []resultsjson.Error{
				{Reason: "detailed diagnosis"},
				{Reason: "Test did not finish"},
			},
		},
	}
	resultCmpOpts := []cmp.Option{
		cmpopts.IgnoreFields(resultsjson.Result{}, "Start", "End"),
		cmpopts.IgnoreFields(resultsjson.Error{}, "Time"),
	}
	if diff := cmp.Diff(got, want, resultCmpOpts...); diff != "" {
		t.Fatalf("Results mismatch (-got +want):\n%s", diff)
	}
}

func TestPreprocessor_TimeoutReached(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		// First test starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "timeout_test"}},
		// Timeout is reached
	}

	logger := logging.NewMultiLogger()
	ctx := logging.AttachLogger(context.Background(), logger)

	hs := newHandlers(resDir, logger, nopPull, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	testCtx, _ := xcontext.WithTimeout(ctx, time.Nanosecond, errors.New("Timeout reached"))
	runProcessor(testCtx, proc, events, errors.New("Timeout reached"))

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(resDir, "tests/timeout_test/log.txt"))
	if err != nil {
		t.Fatalf("Failed to read log.txt: %v", err)
	}

	got := string(b)
	want, _ := regexp.Compile(`Test did not finish\(~[0-9.e\-+]* seconds\) due to Tast command timeout\([0-9.e\-+]* seconds\)`)
	if !want.MatchString(got) {
		t.Errorf("Log doesn't contain an expected message: got %q, want %q", got, want)
	}
}
