// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"

	"chromiumos/tast/internal/protocol"
)

// HandleStack handles fixture stack operation. It should handle requests to
// operate remote fixture stack.
type HandleStack func(ctx context.Context, op *protocol.StackOperationRequest) *protocol.StackOperationResponse

// stackOperationHandler handles stack operations.
type stackOperationHandler struct {
	baseHandler
	handle HandleStack
}

var _ Handler = &stackOperationHandler{}

// NewStackOperationHandler creates a handler which handles stack operations.
func NewStackOperationHandler(handle HandleStack) *stackOperationHandler {
	return &stackOperationHandler{
		handle: handle,
	}
}

func (h *stackOperationHandler) StackOperation(ctx context.Context, req *protocol.StackOperationRequest) *protocol.StackOperationResponse {
	return h.handle(ctx, req)
}
