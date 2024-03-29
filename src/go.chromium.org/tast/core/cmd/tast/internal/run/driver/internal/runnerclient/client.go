// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runnerclient provides a client interface to tast test runners.
package runnerclient

import (
	"context"

	"go.chromium.org/tast/core/internal/minidriver/processor"
	"go.chromium.org/tast/core/internal/protocol"
)

// RunTestsOutput is implemented by callers of RunTests to receive test
// execution events.
//
// Its methods (except RunStart and RunEnd) are called on receiving a
// corresponding test execution event. In case of errors, they can be called in
// an inconsistent way (e.g. EntityEnd is not called after EntityStart due to a
// test crash). RunTestsOutput implementations must be prepared to handle such
// error cases correctly.
//
// All methods except RunEnd can return an error, which leads to immediate
// abort of the test execution and subsequent RunEnd call.
type RunTestsOutput interface {
	// RunStart is called exactly once at the beginning of an overall test
	// execution.
	RunStart(ctx context.Context) error

	EntityStart(ctx context.Context, ev *protocol.EntityStartEvent) error
	EntityLog(ctx context.Context, ev *protocol.EntityLogEvent) error
	EntityError(ctx context.Context, ev *protocol.EntityErrorEvent) error
	EntityEnd(ctx context.Context, ev *protocol.EntityEndEvent) error
	RunLog(ctx context.Context, ev *protocol.RunLogEvent) error

	// RunEnd is called exactly once at the end of an overall test execution.
	// If any other method returns a non-nil error, test execution is aborted
	// immediately and RunEnd is called with the error.
	RunEnd(ctx context.Context, err error)
}

var _ RunTestsOutput = &processor.Processor{}
