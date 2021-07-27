// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"log/syslog"
	"os"
	"path/filepath"
	"time"
)

// SyslogLogger is a Logger that routes logs to syslog.
type SyslogLogger struct {
	w *syslog.Writer
}

var _ Logger = &SyslogLogger{}

// NewSyslogLogger creates a new SyslogLogger.
// It returns an error if it fails to connect to the syslog endpoint.
func NewSyslogLogger() (*SyslogLogger, error) {
	w, err := syslog.New(syslog.LOG_DEBUG, filepath.Base(os.Args[0]))
	if err != nil {
		return nil, err
	}
	return &SyslogLogger{w}, nil
}

// Close closes the underlying connection to the syslog endpoint.
func (l *SyslogLogger) Close() error {
	return l.w.Close()
}

// Log sends a log to syslog.
func (l *SyslogLogger) Log(level Level, ts time.Time, msg string) {
	switch level {
	case LevelInfo:
		l.w.Info(msg)
	case LevelDebug:
		l.w.Debug(msg)
	}
}
