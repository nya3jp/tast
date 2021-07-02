// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package loggingtest provides logging utilities for unit tests.
package loggingtest

import (
	"strings"
	"sync"
	"testing"
	"time"

	"chromiumos/tast/internal/logging"
)

// Logger is a logging.Logger that accumulates logs to an in-memory buffer,
// as well as emitting them as unit test logs.
//
// This is useful for unit tests that inspect logs from a function call.
type Logger struct {
	t     *testing.T
	level logging.Level

	mu   sync.Mutex
	logs []string
}

// NewLogger creates a new Logger.
func NewLogger(t *testing.T, level logging.Level) *Logger {
	return &Logger{t: t, level: level}
}

// Log gets called for a log event.
func (l *Logger) Log(level logging.Level, ts time.Time, msg string) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.t.Log(msg)
	if level >= l.level {
		l.logs = append(l.logs, msg)
	}
}

// Logs returns a list of logs received so far.
func (l *Logger) Logs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.logs...)
}

// String returns received logs as a newline-separated string.
func (l *Logger) String() string {
	return strings.Join(l.Logs(), "\n")
}
