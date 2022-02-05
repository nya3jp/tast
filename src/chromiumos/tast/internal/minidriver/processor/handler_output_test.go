// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/processor"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/testutil"
)

func TestCopyOutputHandler(t *testing.T) {
	tmpDir := t.TempDir()

	resDir := filepath.Join(tmpDir, "results")
	fixtureOutDir := filepath.Join(tmpDir, "out.fixture")
	testOutDir := filepath.Join(tmpDir, "out.test")
	for _, dir := range []string{resDir, fixtureOutDir, testOutDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := testutil.WriteFiles(fixtureOutDir, map[string]string{
		"output.txt":     "fixture output",
		"images/cat.png": "meow",
	}); err != nil {
		t.Fatal(err)
	}

	if err := testutil.WriteFiles(testOutDir, map[string]string{
		"output.txt":     "test output",
		"images/dog.png": "bowwow",
	}); err != nil {
		t.Fatal(err)
	}

	events := []protocol.Event{
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}, OutDir: fixtureOutDir},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test"}, OutDir: testOutDir},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test"},
		&protocol.EntityCopyEndEvent{EntityName: "test"},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "skip"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "skip", Skip: &protocol.Skip{Reasons: []string{"somehow"}}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
		&protocol.EntityCopyEndEvent{EntityName: "fixture"},
	}

	hs := newHandlers(resDir, logging.NewMultiLogger(), os.Rename, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs)
	runProcessor(context.Background(), proc, events, nil)

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	files, err := testutil.ReadFiles(resDir)
	if err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{
		"fixtures/fixture/output.txt":     "fixture output",
		"fixtures/fixture/images/cat.png": "meow",
		"tests/test/output.txt":           "test output",
		"tests/test/images/dog.png":       "bowwow",
	} {
		if got := files[path]; got != want {
			t.Errorf("%s mismatch: got %q, want %q", path, got, want)
		}
	}

	// No file should be copied for OutDir-less entity.
	for path := range files {
		if strings.HasPrefix(path, "tests/skip/") && path != "tests/skip/log.txt" {
			t.Errorf("Excess file found: %s", path)
		}
	}
}
