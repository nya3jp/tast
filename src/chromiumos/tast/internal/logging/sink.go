// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// SinkLogger is a Logger that processes logs by a Sink.
type SinkLogger struct {
	level     Level
	timestamp bool
	sink      Sink
}

// NewSinkLogger creates a new SinkLogger.
//
// level specifies the minimum level of logs the sink should get notified of.
// If timestamp is true, a timestamp is prepended to a log before it is sent to
// the sink.
func NewSinkLogger(level Level, timestamp bool, sink Sink) *SinkLogger {
	return &SinkLogger{
		level:     level,
		timestamp: timestamp,
		sink:      sink,
	}
}

// Log sends a log to the associated sink.
func (l *SinkLogger) Log(level Level, ts time.Time, msg string) {
	if level < l.level {
		return
	}
	if l.timestamp {
		msg = ts.UTC().Format("2006-01-02T15:04:05.000000Z ") + msg
	}
	l.sink.Log(msg)
}

// Sink represents a destination of logs, e.g. a log file or console.
type Sink interface {
	// Log gets called for a log entry.
	Log(msg string)
}

// FuncSink is a Sink that calls a function.
//
// All calls to the underlying function are synchronized.
type FuncSink struct {
	f  func(msg string)
	mu sync.Mutex
}

// NewFuncSink creates a new FuncSink from a function.
func NewFuncSink(f func(msg string)) *FuncSink {
	return &FuncSink{f: f}
}

// Log consumes a log as a function call.
func (s *FuncSink) Log(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.f(msg)
}

// WriterSink is a Sink that writes logs to io.Writer.
//
// All writes to io.Writer are synchronized.
type WriterSink struct {
	w  io.Writer
	mu sync.Mutex
}

// NewWriterSink creates a new WriterSink from io.Writer.
func NewWriterSink(w io.Writer) *WriterSink {
	return &WriterSink{w: w}
}

// Log writes a log to the underlying io.Writer.
func (s *WriterSink) Log(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintln(s.w, msg)
}
