// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"

	"chromiumos/tast/internal/minidriver/failfast"
	"chromiumos/tast/internal/protocol"
)

// failFastHandler aborts test execution if tests fail too often.
type failFastHandler struct {
	baseHandler
	counter *failfast.Counter
}

var _ Handler = &failFastHandler{}

// NewFailFastHandler creates a handler which aborts test execution if tests
// fail too often.
func NewFailFastHandler(counter *failfast.Counter) *failFastHandler {
	return &failFastHandler{counter: counter}
}

func (h *failFastHandler) RunStart(ctx context.Context) error {
	if err := h.counter.Check(); err != nil {
		return err
	}
	return nil
}

func (h *failFastHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	if ei.Entity.Type != protocol.EntityType_TEST {
		return nil
	}
	if len(r.Errors) > 0 {
		h.counter.Increment()
		if err := h.counter.Check(); err != nil {
			return newFatalError(err)
		}
	}
	return nil
}
