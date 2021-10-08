// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/reporting"
	"chromiumos/tast/internal/protocol"
)

// rpcResultsHandler streams test results by gRPC.
type rpcResultsHandler struct {
	baseHandler
	client *reporting.RPCClient
}

var _ handler = &rpcResultsHandler{}

func newRPCResultsHandler(client *reporting.RPCClient) *rpcResultsHandler {
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
		// TODO(b/187793617): Suppress retries in this case.
		return err
	}
	return nil
}
