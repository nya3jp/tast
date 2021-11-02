// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"sync"
	"time"

	"chromiumos/tast/internal/logging"
)

type arrayLogger struct {
	mu   sync.Mutex
	logs []string
}

func (l *arrayLogger) Log(level logging.Level, ts time.Time, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, msg)
}

func (l *arrayLogger) Logs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.logs...)
}

func newArrayLogger() *arrayLogger {
	return &arrayLogger{}
}
