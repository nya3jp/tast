// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

type passThroughHandler struct {
	mu      sync.Mutex
	pass    func(*protocol.RunTestsResponse) error
	pull    func(src, dest string) error
	pullers sync.WaitGroup
}

var _ Handler = &passThroughHandler{}

// NewCopyAndPassThroughHandler creates a handler which copies bundle output
// files, and passes bundle messages to the pass callback. It stalls
// EntityCopyEnd message until file copies are finished.
func NewCopyAndPassThroughHandler(pass func(*protocol.RunTestsResponse) error, pull func(srv, dest string) error) *passThroughHandler {
	return &passThroughHandler{
		pass: pass,
		pull: pull,
	}
}

func (h *passThroughHandler) RunStart(ctx context.Context) error {
	return nil
}

func (h *passThroughHandler) EntityStart(ctx context.Context, ei *entityInfo) error {
	ts, err := ptypes.TimestampProto(ei.Start)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pass(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_EntityStart{
			EntityStart: &protocol.EntityStartEvent{
				Time:   ts,
				Entity: ei.Entity,
				OutDir: ei.FinalOutDir,
			},
		},
	})
}

func (h *passThroughHandler) EntityLog(ctx context.Context, ei *entityInfo, l *logEntry) error {
	ts, err := ptypes.TimestampProto(l.Time)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pass(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_EntityLog{
			EntityLog: &protocol.EntityLogEvent{
				Time:       ts,
				EntityName: ei.Entity.GetName(),
				Text:       l.Text,
			},
		},
	})
}

func (h *passThroughHandler) EntityError(ctx context.Context, ei *entityInfo, e *errorEntry) error {
	ts, err := ptypes.TimestampProto(e.Time)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pass(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_EntityError{
			EntityError: &protocol.EntityErrorEvent{
				Time:       ts,
				EntityName: ei.Entity.GetName(),
				Error:      e.Error,
			},
		},
	})
}

func (h *passThroughHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	ts, err := ptypes.TimestampProto(r.End)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pass(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_EntityEnd{
			EntityEnd: &protocol.EntityEndEvent{
				Time:       ts,
				EntityName: ei.Entity.GetName(),
				Skip:       r.Skip,
				TimingLog:  r.TimingLog,
			},
		},
	})
}

func (h *passThroughHandler) EntityCopyEnd(ctx context.Context, ei *entityInfo) error {
	h.pullers.Add(1)
	go func() {
		// Pull finished test output files in a separate goroutine.
		defer h.pullers.Done()
		// IntermediateOutDir can be empty for skipped tests.
		if ei.IntermediateOutDir != "" {
			if err := moveTestOutputData(h.pull, ei.IntermediateOutDir, ei.FinalOutDir); err != nil {
				// This may be written to a log of an irrelevant test.
				logging.Infof(ctx, "Failed to copy output data of %s: %v", ei.Entity.GetName(), err)
			}
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		if err := h.pass(&protocol.RunTestsResponse{
			Type: &protocol.RunTestsResponse_EntityCopyEnd{
				EntityCopyEnd: &protocol.EntityCopyEndEvent{
					EntityName: ei.Entity.Name,
				},
			},
		}); err != nil {
			logging.Infof(ctx, "Error: failed to pass-through EntityCopyEnd")
		}
	}()
	return nil
}

func (h *passThroughHandler) RunLog(ctx context.Context, l *logEntry) error {
	ts, err := ptypes.TimestampProto(l.Time)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pass(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_RunLog{
			RunLog: &protocol.RunLogEvent{
				Time: ts,
				Text: l.Text,
			},
		},
	})
}

func (h *passThroughHandler) RunEnd(ctx context.Context) {
	// Wait for output file pullers to finish.
	h.pullers.Wait()
}

func (h *passThroughHandler) StackOperation(context.Context, *protocol.StackOperationRequest) *protocol.StackOperationResponse {
	return nil
}
