// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/processor"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/timing"
)

func TestTimingHandler(t *testing.T) {
	resDir := t.TempDir()

	fixtureTiming := &protocol.TimingLog{Root: &protocol.TimingStage{
		Name: "*fixture*",
		Children: []*protocol.TimingStage{
			{Name: "SetUp"},
			{Name: "TearDown"},
		},
	}}
	testTiming := &protocol.TimingLog{Root: &protocol.TimingStage{
		Name: "*test*",
		Children: []*protocol.TimingStage{
			{Name: "login"},
		},
	}}

	events := []protocol.Event{
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test"}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test", TimingLog: testTiming},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture", TimingLog: fixtureTiming},
	}

	log := timing.NewLog()
	ctx := timing.NewContext(context.Background(), log)

	hs := newHandlers(resDir, logging.NewMultiLogger(), nopPull, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	runProcessor(ctx, proc, events, nil)

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	got, err := log.Proto()
	if err != nil {
		t.Fatal(err)
	}

	want := &protocol.TimingLog{Root: &protocol.TimingStage{Children: []*protocol.TimingStage{
		{
			Name: "test",
			Children: []*protocol.TimingStage{
				{Name: "login"},
			},
		},
		// Fixture's timing info is currently not recorded.
	}}}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.IgnoreFields(&protocol.TimingStage{}, "start_time", "end_time")); diff != "" {
		t.Errorf("Timing logs mismatch (-got +want):\n%s", diff)
	}
}
