// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/resultsjson"
)

type entityInfo struct {
	Entity             *protocol.Entity
	Start              time.Time
	IntermediateOutDir string
	FinalOutDir        string
}

type logEntry struct {
	Time time.Time
	Text string
}

type errorEntry struct {
	Time  time.Time
	Error *protocol.Error
}

type entityResult struct {
	Start     time.Time
	End       time.Time
	Skip      *protocol.Skip
	Errors    []*errorEntry
	TimingLog *protocol.TimingLog
}

func newResult(ei *entityInfo, r *entityResult) (*resultsjson.Result, error) {
	if ei.Entity.GetType() != protocol.EntityType_TEST {
		return nil, errors.Errorf("BUG: cannot create result for %v", ei.Entity.GetType())
	}

	test, err := resultsjson.NewTest(ei.Entity)
	if err != nil {
		return nil, err
	}

	var es []resultsjson.Error
	for _, e := range r.Errors {
		es = append(es, resultsjson.Error{
			Time:   e.Time,
			Reason: e.Error.GetReason(),
			File:   e.Error.GetLocation().GetFile(),
			Line:   int(e.Error.GetLocation().GetLine()),
			Stack:  e.Error.GetLocation().GetStack(),
		})
	}

	return &resultsjson.Result{
		Test:       *test,
		Errors:     es,
		Start:      r.Start,
		End:        r.End,
		OutDir:     ei.FinalOutDir,
		SkipReason: strings.Join(r.Skip.GetReasons(), ", "),
	}, nil
}

// fatalError is an error returned by handler when it saw a fatal error and the
// caller should not retry test execution.
type fatalError struct {
	*errors.E
}

func newFatalError(err error) *fatalError {
	return &fatalError{E: errors.Wrap(err, "terminating test execution")}
}

// Handler handles processor events.
type Handler interface {
	RunStart(ctx context.Context) error
	EntityStart(ctx context.Context, ei *entityInfo) error
	EntityLog(ctx context.Context, ei *entityInfo, l *logEntry) error
	EntityError(ctx context.Context, ei *entityInfo, e *errorEntry) error
	EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error
	EntityCopyEnd(ctx context.Context, ei *entityInfo) error
	RunLog(ctx context.Context, l *logEntry) error
	RunEnd(ctx context.Context)
	StackOperation(ctx context.Context, req *protocol.StackOperationRequest) *protocol.StackOperationResponse
}

// baseHandler is an implementation of handler that does nothing in all methods.
// It can be embedded to handler implementations to provide default method
// implementations.
type baseHandler struct{}

var _ Handler = baseHandler{}

func (baseHandler) RunStart(ctx context.Context) error {
	return nil
}

func (baseHandler) EntityStart(ctx context.Context, ei *entityInfo) error {
	return nil
}

func (baseHandler) EntityLog(ctx context.Context, ei *entityInfo, l *logEntry) error {
	return nil
}

func (baseHandler) EntityError(ctx context.Context, ei *entityInfo, e *errorEntry) error {
	return nil
}

func (baseHandler) EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error {
	return nil
}

func (baseHandler) EntityCopyEnd(ctx context.Context, ei *entityInfo) error {
	return nil
}

func (baseHandler) RunLog(ctx context.Context, l *logEntry) error {
	return nil
}

func (baseHandler) RunEnd(ctx context.Context) {}

func (baseHandler) StackOperation(context.Context, *protocol.StackOperationRequest) *protocol.StackOperationResponse {
	return nil
}
