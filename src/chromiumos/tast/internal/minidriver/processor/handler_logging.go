// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/reporting"
)

const testOutputTimeFmt = "15:04:05.000" // format for timestamps attached to test output

// loggingHandler emits logs for test execution events.
type loggingHandler struct {
	baseHandler
	resDir      string
	multiplexer *logging.MultiLogger
	client      *reporting.RPCClient

	loggers []*entityLogger
}

type entityLogger struct {
	Logger *logging.SinkLogger
	File   *os.File
}

var _ Handler = &loggingHandler{}

// NewLoggingHandler creates a new loggingHandler.
// multiplexer should be a MultiLogger all logs from the processor (including
// the preprocessor and all handlers) are sent to; in other words, multiplexer
// should be attached to the context passed to Processor method calls.
// loggingHandler will add/remove additional loggers to/from multiplexer to save
// per-entity logs.
func NewLoggingHandler(resDir string, multiplexer *logging.MultiLogger, client *reporting.RPCClient) *loggingHandler {
	return &loggingHandler{
		resDir:      resDir,
		multiplexer: multiplexer,
		client:      client,
	}
}

func (h *loggingHandler) EntityStart(ctx context.Context, ei *entityInfo) error {
	f, err := os.Create(filepath.Join(ei.FinalOutDir, "log.txt"))
	if err != nil {
		return err
	}

	writers := []io.Writer{f}
	if ei.Entity.GetType() == protocol.EntityType_TEST {
		relPath, err := filepath.Rel(h.resDir, f.Name())
		if err != nil {
			return err
		}
		writers = append(writers, h.client.NewTestLogWriter(ei.Entity.GetName(), relPath))
	}

	logger := &entityLogger{
		Logger: logging.NewSinkLogger(logging.LevelDebug, true, logging.NewWriterSink(io.MultiWriter(writers...))),
		File:   f,
	}
	h.loggers = append(h.loggers, logger)
	h.multiplexer.AddLogger(logger.Logger)

	logging.Infof(ctx, "Started %s %s", entityTypeName(ei.Entity.GetType()), ei.Entity.GetName())
	return nil
}

func (h *loggingHandler) EntityLog(ctx context.Context, ei *entityInfo, l *logEntry) error {
	logging.Infof(ctx, "[%s] %s", l.Time.Format(testOutputTimeFmt), l.Text)
	return nil
}

func (h *loggingHandler) EntityError(ctx context.Context, ei *entityInfo, e *errorEntry) error {
	ts := e.Time.Format(testOutputTimeFmt)
	loc := e.Error.GetLocation()
	if loc == nil {
		logging.Infof(ctx, "[%s] Error: %s", ts, e.Error.GetReason())
	} else {
		logging.Infof(ctx, "[%s] Error at %s:%d: %s", ts, filepath.Base(loc.GetFile()), loc.GetLine(), e.Error.GetReason())
	}
	if stack := loc.GetStack(); stack != "" {
		logging.Infof(ctx, "[%s] Stack trace:\n%s", ts, stack)
	}
	return nil
}

func (h *loggingHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	if reasons := r.Skip.GetReasons(); len(reasons) > 0 {
		logging.Infof(ctx, "Skipped test %s due to missing dependencies: %s", ei.Entity.GetName(), strings.Join(reasons, ", "))
		return nil
	}
	logging.Infof(ctx,
		"Completed %s %s in %v with %d error(s)",
		entityTypeName(ei.Entity.GetType()),
		ei.Entity.GetName(),
		r.End.Sub(r.Start).Round(time.Millisecond),
		len(r.Errors))

	logger := h.loggers[len(h.loggers)-1]
	h.multiplexer.RemoveLogger(logger.Logger)
	logger.File.Close()
	h.loggers = h.loggers[:len(h.loggers)-1]
	return nil
}

func (h *loggingHandler) RunLog(ctx context.Context, l *logEntry) error {
	logging.Infof(ctx, "[%s] %s", l.Time.Format(testOutputTimeFmt), l.Text)
	return nil
}

func entityTypeName(t protocol.EntityType) string {
	switch t {
	case protocol.EntityType_TEST:
		return "test"
	case protocol.EntityType_FIXTURE:
		return "fixture"
	default:
		return fmt.Sprintf("unknown entity type (%d)", int(t))
	}
}
