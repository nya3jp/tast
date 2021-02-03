// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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

// simpleLogger is a basic implementation of the Logger interface that uses log.Logger.
type simpleLogger struct {
	loggerCommon
	l       *logWriter
	verbose bool
	mutex   sync.Mutex // protects l and c
}

// NewSimple returns an object implementing the Logger interface to perform
// simple logging to w. If datetime is true, a timestamp will be appended at the
// begenning of a line. If verbose is true, all messages will be logged to w;
// otherwise, only non-debug messages will be logged to w.
func NewSimple(w io.Writer, datetime, verbose bool) Logger {
	return &simpleLogger{
		l:       newLogWriter(w, datetime),
		verbose: verbose,
	}
}

func (s *simpleLogger) Close() error { return nil }

func (s *simpleLogger) Log(args ...interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.l.Print(args...)
	s.loggerCommon.print(args...)
}

func (s *simpleLogger) Logf(format string, args ...interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.l.Printf(format, args...)
	s.loggerCommon.printf(format, args...)
}

func (s *simpleLogger) Debug(args ...interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.verbose {
		s.l.Print(args...)
	}
	s.loggerCommon.print(args...)
}

func (s *simpleLogger) Debugf(format string, args ...interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.verbose {
		s.l.Printf(format, args...)
	}
	s.loggerCommon.printf(format, args...)
}

// NewDiscard is a convencience function that returns a Logger that discards all messages.
func NewDiscard() Logger {
	return NewSimple(ioutil.Discard, false, false)
}
