// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package output defines output stream from entities.
package output

import (
	"context"
	"sync"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

// Stream is an interface to report streamed outputs of multiple entity runs.
// Note that testing.Stream is for a single entity in contrast.
type Stream interface {
	// EntityStart reports that an entity has started.
	EntityStart(ei *protocol.Entity, outDir string) error
	// EntityLog reports an informational log message.
	EntityLog(ei *protocol.Entity, msg string) error
	// EntityError reports an error from an entity. An entity that reported one or more errors should be considered failure.
	EntityError(ei *protocol.Entity, e *protocol.Error) error
	// EntityEnd reports that an entity has ended. If skipReasons is not empty it is considered skipped.
	EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error
	// ExternalEvent reports events happened in external bundles.
	ExternalEvent(res *protocol.RunTestsResponse) error
	// StackOperation reports stack operation request.
	StackOperation(ctx context.Context, req *protocol.StackOperationRequest) (*protocol.StackOperationResponse, error)
}

// EntityStream wraps planner.OutputStream for a single entity.
//
// EntityStream implements testing.OutputStream. EntityStream is goroutine-safe;
// it is safe to call its methods concurrently from multiple goroutines.
type EntityStream struct {
	out Stream
	ei  *protocol.Entity

	mu    sync.Mutex
	errs  []*protocol.Error
	ended bool
}

var _ testing.OutputStream = &EntityStream{}

// NewEntityStream creates entityOutputStream for out and ei.
func NewEntityStream(out Stream, ei *protocol.Entity) *EntityStream {
	return &EntityStream{out: out, ei: ei}
}

var errAlreadyEnded = errors.New("test has already ended")

// Start reports that the test has started. It should be called exactly once.
func (w *EntityStream) Start(outDir string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	if w.ei.Name == "" {
		return nil
	}
	return w.out.EntityStart(w.ei, outDir)
}

// Log reports an informational log from the entity.
func (w *EntityStream) Log(msg string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	if w.ei.Name == "" {
		// TODO(crbug.com/1035940): Consider emitting RunLog.
		return nil
	}
	return w.out.EntityLog(w.ei, msg)
}

// Log reports an error from the entity.
func (w *EntityStream) Error(e *protocol.Error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	w.errs = append(w.errs, e)
	if w.ei.Name == "" {
		// TODO(crbug.com/1035940): Consider emitting RunError.
		return nil
	}
	return w.out.EntityError(w.ei, e)
}

// End reports that the entity has ended. After End is called, all methods will
// fail with an error.
func (w *EntityStream) End(skipReasons []string, timingLog *timing.Log) error {
	if timingLog == nil {
		panic("BUG: entityOutputStream.End: nil timing log")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	if w.ei.Name == "" {
		return nil
	}
	w.ended = true
	return w.out.EntityEnd(w.ei, skipReasons, timingLog)
}

// Errors returns errors reported so far.
func (w *EntityStream) Errors() []*protocol.Error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// We always append to errs, so it is safe to return without copy.
	return w.errs
}
