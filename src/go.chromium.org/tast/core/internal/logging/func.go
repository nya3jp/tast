// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"sync"
	"time"
)

// FuncLogger is a Logger that calls a function.
//
// All calls to the underlying function are synchronized.
type FuncLogger struct {
	f  func(level Level, ts time.Time, msg string)
	mu sync.Mutex
}

// NewFuncLogger creates a new FuncLogger.
func NewFuncLogger(f func(level Level, ts time.Time, msg string)) *FuncLogger {
	return &FuncLogger{
		f: f,
	}
}

// Log sends a log to the associated sink.
func (l *FuncLogger) Log(level Level, ts time.Time, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.f(level, ts, msg)
}
