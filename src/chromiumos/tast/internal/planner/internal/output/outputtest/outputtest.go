// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package outputtest provides functionalities for unit testing output package.
package outputtest

import (
	"context"

	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

// Sink is fake output sink for unit testing.
// It implements output.Stream .
type Sink struct {
	msgs []protocol.Event
}

var _ output.Stream = &Sink{}

// NewSink creates a Sink.
func NewSink() *Sink {
	return &Sink{}
}

// RunLog implements output.Stream.
func (s *Sink) RunLog(msg string) error {
	s.msgs = append(s.msgs, &protocol.RunLogEvent{
		Text: msg,
	})
	return nil
}

// EntityStart implements output.Stream.
func (s *Sink) EntityStart(ei *protocol.Entity, outDir string) error {
	s.msgs = append(s.msgs, &protocol.EntityStartEvent{
		Entity: ei,
		OutDir: outDir,
	})
	return nil
}

// EntityLog implements output.Stream.
func (s *Sink) EntityLog(ei *protocol.Entity, msg string) error {
	s.msgs = append(s.msgs, &protocol.EntityLogEvent{
		EntityName: ei.GetName(),
		Text:       msg,
	})
	return nil
}

// EntityError implements output.Stream.
func (s *Sink) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	s.msgs = append(s.msgs, &protocol.EntityErrorEvent{
		EntityName: ei.GetName(),
		// Clear Error fields except for Reason.
		Error: &protocol.Error{Reason: e.GetReason()},
	})
	return nil
}

// EntityEnd implements output.Stream.
func (s *Sink) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	// Drop timingLog.
	var skip *protocol.Skip
	if len(skipReasons) > 0 {
		skip = &protocol.Skip{Reasons: skipReasons}
	}
	s.msgs = append(s.msgs, &protocol.EntityEndEvent{EntityName: ei.GetName(), Skip: skip})
	return nil
}

// ExternalEvent implements output.Stream.
func (s *Sink) ExternalEvent(res *protocol.RunTestsResponse) error {
	return nil
}

// StackOperation implements output.Stream.
func (s *Sink) StackOperation(ctx context.Context, req *protocol.StackOperationRequest) (*protocol.StackOperationResponse, error) {
	return nil, nil
}

// ReadAll reads all control messages written to the sink.
func (s *Sink) ReadAll() []protocol.Event {
	return s.msgs
}
