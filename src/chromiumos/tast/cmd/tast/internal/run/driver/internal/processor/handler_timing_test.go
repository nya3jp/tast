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
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
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

	proc := processor.New(resDir, logging.NewMultiLogger(), nopDiagnose, nopPull, nil, nil)
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
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(protocol.TimingStage{}, "StartTime", "EndTime")); diff != "" {
		t.Errorf("Timing logs mismatch (-got +want):\n%s", diff)
	}
}
