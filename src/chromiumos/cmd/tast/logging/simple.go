// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"io"
	"log"
	"sync"
)

// simpleLogger is a basic implementation of the Logger interface that uses log.Logger.
type simpleLogger struct {
	loggerCommon
	l       *log.Logger
	verbose bool
	mutex   sync.Mutex // protects l and c
}

// NewSimple returns an object implementing the Logger interface to perform
// simple logging to w. flag contains logging properties to be passed to log.New.
// If verbose is true, all messages will be logged to w; otherwise, only non-debug
// messages will be logged to w.
func NewSimple(w io.Writer, flag int, verbose bool) Logger {
	return &simpleLogger{
		loggerCommon: make(loggerCommon),
		l:            log.New(w, "", flag),
		verbose:      verbose,
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

func (s *simpleLogger) Status(msg string) {}
