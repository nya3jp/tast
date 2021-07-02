// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"sync"
	"time"
)

// Level indicates a logging level. A larger level value means a log is more
// important.
type Level int

const (
	// LevelDebug represents the DEBUG level.
	LevelDebug Level = iota
	// LevelInfo represents the INFO level.
	LevelInfo
)

// Logger defines the interface for loggers that consume logs sent via
// context.Context.
//
// You can create a new context with a Logger attached by AttachLogger. The
// attached logger will consume all logs sent to the context, as well as those
// logs sent to its descendant contexts as long as you don't attach another
// logger to a descendant context with no propagation. See AttachLogger for more
// details.
type Logger interface {
	// Log gets called for a log entry.
	Log(level Level, ts time.Time, msg string)
}

// MultiLogger is a Logger that copies logs to multiple underlying loggers.
// A logger can be added and removed from MultiLogger at any time.
type MultiLogger struct {
	mu      sync.Mutex
	loggers []Logger
}

// NewMultiLogger creates a new MultiLogger with a specified initial set of
// underlying loggers.
func NewMultiLogger(loggers ...Logger) *MultiLogger {
	return &MultiLogger{loggers: loggers}
}

// Log copies a log to the current underlying loggers.
func (ml *MultiLogger) Log(level Level, ts time.Time, msg string) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	for _, logger := range ml.loggers {
		logger.Log(level, ts, msg)
	}
}

// AddLogger adds a logger to the set of underlying loggers.
func (ml *MultiLogger) AddLogger(logger Logger) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.loggers = append(ml.loggers, logger)
}

// RemoveLogger removes a logger from the set of underlying loggers.
func (ml *MultiLogger) RemoveLogger(logger Logger) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	j := 0
	for i, l := range ml.loggers {
		if l == logger {
			continue
		}
		ml.loggers[j] = ml.loggers[i]
		j++
	}
	ml.loggers = ml.loggers[:j]
}
