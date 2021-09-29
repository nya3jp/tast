// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"strings"

	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// resultsHandler collects test results.
type resultsHandler struct {
	baseHandler
	results []*resultsjson.Result
}

var _ handler = &resultsHandler{}

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

	test, err := resultsjson.NewTest(ei.Entity)
	if err != nil {
		logging.Info(ctx, "Failed converting protocol.Entity to resultsjson.Test: ", err)
	}

	var errors []resultsjson.Error
	for _, e := range r.Errors {
		errors = append(errors, resultsjson.Error{
			Time:   e.Time,
			Reason: e.Error.GetReason(),
			File:   e.Error.GetLocation().GetFile(),
			Line:   int(e.Error.GetLocation().GetLine()),
			Stack:  e.Error.GetLocation().GetStack(),
		})
	}

	h.results = append(h.results, &resultsjson.Result{
		Test:       *test,
		Errors:     errors,
		Start:      r.Start,
		End:        r.End,
		OutDir:     ei.FinalOutDir,
		SkipReason: strings.Join(r.Skip.GetReasons(), ", "),
	})
	return nil
}
