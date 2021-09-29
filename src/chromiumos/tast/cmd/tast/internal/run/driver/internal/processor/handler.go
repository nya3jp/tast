// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"time"

	"chromiumos/tast/internal/protocol"
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

type handler interface {
	RunStart(ctx context.Context) error
	EntityStart(ctx context.Context, ei *entityInfo) error
	EntityLog(ctx context.Context, ei *entityInfo, l *logEntry) error
	EntityError(ctx context.Context, ei *entityInfo, e *errorEntry) error
	EntityEnd(ctx context.Context, ei *entityInfo, r *entityResult) error
	RunLog(ctx context.Context, l *logEntry) error
	RunEnd(ctx context.Context)
}

// baseHandler is an implementation of handler that does nothing in all methods.
// It can be embedded to handler implementations to provide default method
// implementations.
type baseHandler struct{}

var _ handler = baseHandler{}

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

func (baseHandler) RunLog(ctx context.Context, l *logEntry) error {
	return nil
}

func (baseHandler) RunEnd(ctx context.Context) {}
