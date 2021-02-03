// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package logging is used by the tast executable to write informational output.
package logging

import (
	"context"
	"io"
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
