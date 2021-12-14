// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"path/filepath"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/reporting"
	"chromiumos/tast/internal/run/resultsjson"
)

// streamedResultsHandler saves results to a file progressively.
type streamedResultsHandler struct {
	baseHandler
	resDir string

	writer *reporting.StreamedWriter
}

var _ Handler = &streamedResultsHandler{}

// NewStreamedResultsHandler creates a handler which saves results to a file
// progressively.
func NewStreamedResultsHandler(resDir string) *streamedResultsHandler {
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

	result, err := newResult(ei, r)
	if err != nil {
		return err
	}

	return h.writer.Write(result, true)
}

func (h *streamedResultsHandler) RunEnd(ctx context.Context) {
	h.writer.Close()
	h.writer = nil
}
