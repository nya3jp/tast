// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package logging is used by the tast executable to write informational output.
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
)

// Logger is the interface used for logging by the tast executable.
type Logger interface {
	// Close deinitializes the logger, returning the terminal to its original state (if necessary).
	Close() error

	// Log formats args using default formatting and logs them unconditionally and permanently (i.e.
	// the message will remain in the terminal's scrollback buffer).
	Log(args ...interface{})
	// Logf is similar to Log but formats args as per fmt.Sprintf.
	Logf(format string, args ...interface{})

	// Debug formats args using default formatting and prints a message that may be omitted in
	// non-verbose modes or only displayed onscreen for a short period of time.
	Debug(args ...interface{})
	// Debugf is similar to Debug but formats args as per fmt.Sprintf.
	Debugf(format string, args ...interface{})

	// AddWriter adds an additional writer to which Log, Logf, Debug, and Debugf's messages are
	// logged (regardless of any verbosity settings).
	// flag contains logging properties to be passed to log.New.
	// An error is returned if w has already been added.
	AddWriter(w io.Writer, flag int) error
	// RemoveWriter stops logging to a writer previously passed to AddWriter.
	// An error is returned if w was not previously added.
	RemoveWriter(w io.Writer) error
}

// Key type for objects attached to context.Context objects.
type contextKeyType string

// Key used for attaching a Logger to a context.Context.
var loggerKey contextKeyType = "logger"

// NewContext returns a new context derived from ctx that carries value lg.
func NewContext(ctx context.Context, lg Logger) context.Context {
	return context.WithValue(ctx, loggerKey, lg)
}

// FromContext returns the Logger value stored in ctx, if any.
func FromContext(ctx context.Context) (Logger, bool) {
	lg, ok := ctx.Value(loggerKey).(Logger)
	return lg, ok
}

// loggerCommon holds state shared between all implementations of the Logger interface.
// The zero value for loggerCommon is ready for use.
// Its methods can be called from multiple goroutines concurrently.
type loggerCommon struct {
	mu sync.Mutex // protects ws
	ws map[io.Writer]*log.Logger
}

// addWriter starts writing to w.
func (c *loggerCommon) AddWriter(w io.Writer, flag int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.ws[w]; ok {
		return fmt.Errorf("writer %v already added", w)
	}
	if c.ws == nil {
		c.ws = make(map[io.Writer]*log.Logger)
	}
	c.ws[w] = log.New(w, "", flag)
	return nil
}

// removeWriter stops writing to w.
func (c *loggerCommon) RemoveWriter(w io.Writer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.ws[w]; !ok {
		return fmt.Errorf("writer %v not registered", w)
	}
	delete(c.ws, w)
	return nil
}

// print formats args using default formatting and writes them to all writers.
func (c *loggerCommon) print(args ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.ws {
		l.Print(args...)
	}
}

// printf formats args as per fmt.Sprintf and writes them to all open files.
func (c *loggerCommon) printf(format string, args ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.ws {
		l.Printf(format, args...)
	}
}
