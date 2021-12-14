// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

// timingHandler records timing information via context.Context.
type timingHandler struct {
	baseHandler
	stage *timing.Stage
}

var _ Handler = &timingHandler{}

// NewTimingHandler creates a handler which records timing information via
// context.Context.
func NewTimingHandler() *timingHandler {
	return &timingHandler{}
}

func (h *timingHandler) EntityStart(ctx context.Context, ei *entityInfo) error {
	if ei.Entity.GetType() != protocol.EntityType_TEST {
		return nil
	}
	if h.stage != nil {
		return errors.New("two tests started concurrently")
	}
	_, h.stage = timing.Start(ctx, ei.Entity.GetName())
	return nil
}

func (h *timingHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	if h.stage == nil {
		return nil
	}
	if log, err := timing.LogFromProto(r.TimingLog); err != nil {
		logging.Infof(ctx, "Failed importing timing log for %v: %v", ei.Entity.GetName(), err)
	} else if err := h.stage.Import(log); err != nil {
		logging.Infof(ctx, "Failed importing timing log for %v: %v", ei.Entity.GetName(), err)
	}
	h.stage.End()
	h.stage = nil
	return nil
}
