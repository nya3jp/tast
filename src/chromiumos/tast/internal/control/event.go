// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package control

import (
	"fmt"
	"log/syslog"
	"path/filepath"
	"time"

	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// EventWriter is used to report test events.
//
// EventWriter is goroutine-safe; it is safe to call its methods concurrently from multiple
// goroutines.
//
// Events are basically written through to MessageWriter, but they are also sent to syslog for
// easier debugging.
type EventWriter struct {
	mw *MessageWriter
	lg *syslog.Writer

	testName string // name of the current test
}

// NewEventWriter constructs a new EventWriter that writes control messages to mw.
func NewEventWriter(mw *MessageWriter) *EventWriter {
	// Continue even if we fail to connect to syslog.
	lg, _ := syslog.New(syslog.LOG_INFO, "tast")
	return &EventWriter{mw: mw, lg: lg}
}

// RunLog writes control.RunLog.
func (ew *EventWriter) RunLog(msg string) error {
	if ew.lg != nil {
		ew.lg.Info(msg)
	}
	return ew.mw.WriteMessage(&RunLog{Time: time.Now(), Text: msg})
}

// TestStart writes control.TestStart.
func (ew *EventWriter) TestStart(t *testing.TestInstance) error {
	ew.testName = t.Name
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== start", t.Name))
	}
	return ew.mw.WriteMessage(&TestStart{Time: time.Now(), Test: *t})
}

// TestLog writes control.TestLog.
func (ew *EventWriter) TestLog(msg string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: %s", ew.testName, msg))
	}
	return ew.mw.WriteMessage(&TestLog{Time: time.Now(), Text: msg})
}

// TestError writes control.TestError.
func (ew *EventWriter) TestError(e *testing.Error) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: Error at %s:%d: %s", ew.testName, filepath.Base(e.File), e.Line, e.Reason))
	}
	return ew.mw.WriteMessage(&TestError{Time: time.Now(), Error: *e})
}

// TestEnd writes control.TestEnd.
func (ew *EventWriter) TestEnd(t *testing.TestInstance, skipReasons []string, timingLog *timing.Log) error {
	ew.testName = ""
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== end", t.Name))
	}

	return ew.mw.WriteMessage(&TestEnd{
		Time:        time.Now(),
		Name:        t.Name,
		SkipReasons: skipReasons,
		TimingLog:   timingLog,
	})
}
