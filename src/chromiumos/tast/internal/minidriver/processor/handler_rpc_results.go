// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/reporting"
)

// rpcResultsHandler streams test results by gRPC.
type rpcResultsHandler struct {
	baseHandler
	client *reporting.RPCClient
}

var _ Handler = &rpcResultsHandler{}

// NewRPCResultsHandler creates a handler which streams test results by gRPC.
func NewRPCResultsHandler(client *reporting.RPCClient) *rpcResultsHandler {
	return &rpcResultsHandler{client: client}
}

func (h *rpcResultsHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	if ei.Entity.GetType() != protocol.EntityType_TEST {
		return nil
	}

	result, err := newResult(ei, r)
	if err != nil {
		return err
	}

	if err := h.client.ReportResult(ctx, result); err != nil {
		if err == reporting.ErrTerminate {
			return newFatalError(err)
		}
		return err
	}
	return nil
}
