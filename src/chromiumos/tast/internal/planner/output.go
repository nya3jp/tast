// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"sync"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

// OutputStream is an interface to report streamed outputs of multiple entity runs.
// Note that testing.OutputStream is for a single entity in contrast.
type OutputStream interface {
	// EntityStart reports that an entity has started.
	EntityStart(ei *protocol.Entity, outDir string) error
	// EntityLog reports an informational log message.
	EntityLog(ei *protocol.Entity, msg string) error
	// EntityError reports an error from an entity. An entity that reported one or more errors should be considered failure.
	EntityError(ei *protocol.Entity, e *protocol.Error) error
	// EntityEnd reports that an entity has ended. If skipReasons is not empty it is considered skipped.
	EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error
}

// entityOutputStream wraps planner.OutputStream for a single entity.
//
// entityOutputStream implements testing.OutputStream. entityOutputStream is goroutine-safe;
// it is safe to call its methods concurrently from multiple goroutines.
type entityOutputStream struct {
	out OutputStream
	ei  *protocol.Entity

	mu    sync.Mutex
	errs  []*protocol.Error
	ended bool
}

var _ testing.OutputStream = &entityOutputStream{}

// newEntityOutputStream creates entityOutputStream for out and ei.
func newEntityOutputStream(out OutputStream, ei *protocol.Entity) *entityOutputStream {
	return &entityOutputStream{out: out, ei: ei}
}

var errAlreadyEnded = errors.New("test has already ended")

// Start reports that the test has started. It should be called exactly once.
func (w *entityOutputStream) Start(outDir string) error {
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
func (w *entityOutputStream) Log(msg string) error {
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
func (w *entityOutputStream) Error(e *protocol.Error) error {
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
func (w *entityOutputStream) End(skipReasons []string, timingLog *timing.Log) error {
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
func (w *entityOutputStream) Errors() []*protocol.Error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// We always append to errs, so it is safe to return without copy.
	return w.errs
}
