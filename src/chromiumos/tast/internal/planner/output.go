// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"sync"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// OutputStream is an interface to report streamed outputs of multiple entity runs.
// Note that testing.OutputStream is for a single entity in contrast.
type OutputStream interface {
	// EntityStart reports that an entity has started.
	EntityStart(ei *testing.EntityInfo, outDir string) error
	// EntityLog reports an informational log message.
	EntityLog(ei *testing.EntityInfo, msg string) error
	// EntityError reports an error from an entity. An entity that reported one or more errors should be considered failure.
	EntityError(ei *testing.EntityInfo, e *testing.Error) error
	// EntityEnd reports that an entity has ended. If skipReasons is not empty it is considered skipped.
	EntityEnd(ei *testing.EntityInfo, skipReasons []string, timingLog *timing.Log) error
}

// entityOutputStream wraps planner.OutputStream for a single entity.
//
// entityOutputStream implements testing.OutputStream. entityOutputStream is goroutine-safe;
// it is safe to call its methods concurrently from multiple goroutines.
type entityOutputStream struct {
	out OutputStream
	ei  *testing.EntityInfo

	mu    sync.Mutex
	ended bool
}

var _ testing.OutputStream = &entityOutputStream{}

// newEntityOutputStream creates entityOutputStream for out and ei.
func newEntityOutputStream(out OutputStream, ei *testing.EntityInfo) *entityOutputStream {
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
	return w.out.EntityStart(w.ei, outDir)
}

// Log reports an informational log from the entity.
func (w *entityOutputStream) Log(msg string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
	}
	return w.out.EntityLog(w.ei, msg)
}

// Log reports an error from the entity.
func (w *entityOutputStream) Error(e *testing.Error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ended {
		return errAlreadyEnded
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
	w.ended = true
	return w.out.EntityEnd(w.ei, skipReasons, timingLog)
}

// nullOutputStream is an implementation of testing.OutputStream that discards
// all logs and errors.
type nullOutputStream struct{}

var _ testing.OutputStream = &nullOutputStream{}

func newNullOutputStream() *nullOutputStream {
	return &nullOutputStream{}
}

func (w *nullOutputStream) Log(msg string) error {
	return nil
}

func (w *nullOutputStream) Error(e *testing.Error) error {
	return nil
}

// teeOutputStream is an implementation of testing.OutputStream that collects
// errors in a slice before forwarding them to a parent OutputStream.
type teeOutputStream struct {
	parent testing.OutputStream

	mu     sync.Mutex
	errors []*testing.Error
}

var _ testing.OutputStream = &teeOutputStream{}

func newTeeOutputStream(parent testing.OutputStream) *teeOutputStream {
	return &teeOutputStream{parent: parent}
}

func (w *teeOutputStream) Log(msg string) error {
	return w.parent.Log(msg)
}

func (w *teeOutputStream) Error(e *testing.Error) error {
	w.mu.Lock()
	w.errors = append(w.errors, e)
	w.mu.Unlock()
	return w.parent.Error(e)
}

// Errors returns a collected list of errors.
func (w *teeOutputStream) Errors() []*testing.Error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]*testing.Error(nil), w.errors...)
}
