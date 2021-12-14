// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/processor"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/resultsjson"
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
	proc := processor.New(resDir, nopDiagnose, hs)
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
	proc := processor.New(resDir, nopDiagnose, hs)
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
		b, err := ioutil.ReadFile(filepath.Join(resDir, tc.relPath))
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

// TestPreprocessor_UnmatchedEntityEvent checks preprocessor's behavior on
// receiving an unmatched EntityLog/EntityError/EntityEnd message.
func TestPreprocessor_UnmatchedEntityEvent(t *testing.T) {
	for _, event := range []protocol.Event{
		&protocol.EntityLogEvent{Time: epochpb, EntityName: "test2"},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "test2"},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test2"},
	} {
		t.Run(fmt.Sprintf("%T", event), func(t *testing.T) {
			resDir := t.TempDir()

			events := []protocol.Event{
				&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test1"}},
				event,
			}

			logger := logging.NewMultiLogger()
			ctx := logging.AttachLogger(context.Background(), logger)

			hs := newHandlers(resDir, logger, nopPull, nil, nil)
			proc := processor.New(resDir, nopDiagnose, hs)
			runProcessor(ctx, proc, events, nil)

			if err := proc.FatalError(); err != nil {
				t.Errorf("Processor had a fatal error: %v", err)
			}

			b, err := ioutil.ReadFile(filepath.Join(resDir, "tests/test1/log.txt"))
			if err != nil {
				t.Fatalf("Failed to read log.txt: %v", err)
			}

			got := string(b)
			const want = "no such entity running: test2"
			if !strings.Contains(got, want) {
				t.Errorf("Log doesn't contain an expected message: got %q, want %q", got, want)
			}
		})
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
	proc := processor.New(resDir, fakeDiagnose, hs)
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
