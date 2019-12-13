// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"fmt"
	"sync"
	"time"

	"chromiumos/tast/cmd/tast/display"
)

type fancyLogger struct {
	loggerCommon
	d      *display.Display
	closed bool
	mutex  sync.Mutex // protects d and closed
}

// NewFancy returns an implementation of the Logger interface that renders messages onscreen using ANSI
// escape codes to create multiple scrolling regions (e.g. a limited number of debug messages remain
// onscreen).
func NewFancy(lines int) (Logger, error) {
	disp, err := display.New(&display.VT100Term{}, lines)
	if err != nil {
		return nil, err
	}
	return &fancyLogger{d: disp}, nil
}

func (f *fancyLogger) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.d.Close()
	f.closed = true
	return nil
}

func (f *fancyLogger) getPrefix() string {
	return time.Now().Format("01/02 15:04:05 ")
}

func (f *fancyLogger) Log(args ...interface{}) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if !f.closed {
		// Write to both sections. The verbose section can be hard to
		// interpret if it doesn't include important messages logged via
		// Log and Logf.
		f.d.AddPersistent(f.getPrefix() + fmt.Sprint(args...))
		f.d.AddVerbose(f.getPrefix() + fmt.Sprint(args...))
		f.loggerCommon.print(args...)
	}
}

func (f *fancyLogger) Logf(format string, args ...interface{}) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if !f.closed {
		f.d.AddPersistent(f.getPrefix() + fmt.Sprintf(format, args...))
		f.d.AddVerbose(f.getPrefix() + fmt.Sprintf(format, args...))
		f.loggerCommon.printf(format, args...)
	}
}

func (f *fancyLogger) Debug(args ...interface{}) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if !f.closed {
		f.d.AddVerbose(f.getPrefix() + fmt.Sprint(args...))
		f.loggerCommon.print(args...)
	}
}

func (f *fancyLogger) Debugf(format string, args ...interface{}) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if !f.closed {
		f.d.AddVerbose(f.getPrefix() + fmt.Sprintf(format, args...))
		f.loggerCommon.printf(format, args...)
	}
}

func (f *fancyLogger) Status(msg string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if !f.closed {
		f.d.SetStatus(msg)
	}
}
