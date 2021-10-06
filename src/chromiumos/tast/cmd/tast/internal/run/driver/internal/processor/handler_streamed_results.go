// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"path/filepath"
	"strings"

	"chromiumos/tast/cmd/tast/internal/run/reporting"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/protocol"
)

// streamedResultsHandler saves results to a file progressively.
type streamedResultsHandler struct {
	baseHandler
	resDir string

	writer *reporting.StreamedWriter
}

var _ handler = &streamedResultsHandler{}

func newStreamedResultsHandler(resDir string) *streamedResultsHandler {
	return &streamedResultsHandler{resDir: resDir}
}

func (h *streamedResultsHandler) RunStart(ctx context.Context) error {
	writer, err := reporting.NewStreamedWriter(filepath.Join(h.resDir, reporting.StreamedResultsFilename))
	if err != nil {
		return err
	}
	h.writer = writer
	return nil
}

func (h *streamedResultsHandler) EntityStart(ctx context.Context, ei *entityInfo) error {
	if ei.Entity.Type != protocol.EntityType_TEST {
		return nil
	}

	t, err := resultsjson.NewTest(ei.Entity)
	if err != nil {
		return err
	}
	return h.writer.Write(&resultsjson.Result{
		Test:   *t,
		Start:  ei.Start,
		OutDir: ei.FinalOutDir,
	}, false)
}

func (h *streamedResultsHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	if ei.Entity.Type != protocol.EntityType_TEST {
		return nil
	}

	t, err := resultsjson.NewTest(ei.Entity)
	if err != nil {
		return err
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
	var skipReason string
	if r.Skip != nil {
		skipReason = strings.Join(r.Skip.Reasons, ", ")
	}
	return h.writer.Write(&resultsjson.Result{
		Test:       *t,
		Start:      r.Start,
		End:        r.End,
		OutDir:     ei.FinalOutDir,
		Errors:     errors,
		SkipReason: skipReason,
	}, true)
}

func (h *streamedResultsHandler) RunEnd(ctx context.Context) {
	h.writer.Close()
	h.writer = nil
}
