// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package logging is used by the tast executable to write informational output.
package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"
)

// logWriter is to write logs into entries into the specified writer with or without timestamp.
type logWriter struct {
	mu       sync.Mutex // ensures atomic writes; protects the following fields
	datetime bool       // whether to append timestamp or not
	out      io.Writer  // destination for output
	buf      []byte     // for accumulating text to write
}

// newLogWriter creates a new writer. The out variable sets the destination of logs. The datetime
// argument determines if the timestamp is appended or not.
func newLogWriter(out io.Writer, datetime bool) *logWriter {
	return &logWriter{out: out, datetime: datetime}
}

func (l *logWriter) Print(v ...interface{}) { l.Output(fmt.Sprint(v...)) }

func (l *logWriter) Printf(format string, v ...interface{}) {
	l.Output(fmt.Sprintf(format, v...))
}

func (l *logWriter) Output(s string) error {
	now := time.Now() // get this early.

	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf = l.buf[:0]
	if l.datetime {
		l.buf = append(l.buf, now.UTC().Format("2006-01-02T15:04:05.000000Z ")...)
	}
	l.buf = append(l.buf, s...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		l.buf = append(l.buf, '\n')
	}
	_, err := l.out.Write(l.buf)
	return err
}

// Logger provides the logging mechanism for Tast CLI.
type Logger struct {
	mu      sync.Mutex // protects ws and log order; must be held on emitting logs
	l       *logWriter
	verbose bool
	ws      map[io.Writer]*logWriter
}

// NewSimple returns an object implementing the Logger interface to perform
// simple logging to w. If datetime is true, a timestamp will be appended at the
// beginning of a line. If verbose is true, all messages will be logged to w;
// otherwise, only non-debug messages will be logged to w.
func NewSimple(w io.Writer, datetime, verbose bool) *Logger {
	return &Logger{
		l:       newLogWriter(w, datetime),
		verbose: verbose,
		ws:      make(map[io.Writer]*logWriter),
	}
}

// Close closes the logger.
func (s *Logger) Close() error { return nil }

// Log formats args using default formatting and logs them unconditionally.
func (s *Logger) Log(args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.l.Print(args...)
	for _, l := range s.ws {
		l.Print(args...)
	}
}

// Logf is similar to Log but formats args as per fmt.Sprintf.
func (s *Logger) Logf(format string, args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.l.Printf(format, args...)
	for _, l := range s.ws {
		l.Printf(format, args...)
	}
}

// Debug formats args using default formatting and prints a message that may be
// omitted in non-verbose modes.
func (s *Logger) Debug(args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.verbose {
		s.l.Print(args...)
	}
	for _, l := range s.ws {
		l.Print(args...)
	}
}

// Debugf is similar to Debug but formats args as per fmt.Sprintf.
func (s *Logger) Debugf(format string, args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.verbose {
		s.l.Printf(format, args...)
	}
	for _, l := range s.ws {
		l.Printf(format, args...)
	}
}

// AddWriter adds an additional writer to which Log, Logf, Debug, and Debugf's
// messages are logged (regardless of any verbosity settings).
// If datetime is true, a timestamp will be appended at the beginning of a line.
// An error is returned if w has already been added.
func (s *Logger) AddWriter(w io.Writer, datetime bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ws[w]; ok {
		return fmt.Errorf("writer %v already added", w)
	}
	s.ws[w] = newLogWriter(w, datetime)
	return nil
}

// RemoveWriter stops logging to a writer previously passed to AddWriter.
// An error is returned if w was not previously added.
func (s *Logger) RemoveWriter(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ws[w]; !ok {
		return fmt.Errorf("writer %v not registered", w)
	}
	delete(s.ws, w)
	return nil
}

// NewDiscard is a convencience function that returns a Logger that discards all messages.
func NewDiscard() *Logger {
	return NewSimple(ioutil.Discard, false, false)
}
