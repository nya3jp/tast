// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/resultsjson"
)

// resultsHandler collects test results.
type resultsHandler struct {
	baseHandler
	results []*resultsjson.Result
}

var _ Handler = &resultsHandler{}

func newResultsHandler() *resultsHandler {
	return &resultsHandler{}
}

// Results returns collected test results.
//
// It is safe to call this method only after finishing whole test execution.
func (h *resultsHandler) Results() []*resultsjson.Result {
	return h.results
}

func (h *resultsHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	if ei.Entity.GetType() != protocol.EntityType_TEST {
		return nil
	}

	result, err := newResult(ei, r)
	if err != nil {
		return err
	}

	h.results = append(h.results, result)
	return nil
}
