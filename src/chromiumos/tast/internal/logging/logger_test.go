// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
)

func TestMultiLogger(t *testing.T) {
	logger1 := loggingtest.NewLogger(t, logging.LevelInfo)
	logger2 := loggingtest.NewLogger(t, logging.LevelInfo)

	logger := logging.NewMultiLogger(logger1)
	logger.Log(logging.LevelInfo, time.Time{}, "aaa")
	logger.AddLogger(logger2)
	logger.Log(logging.LevelInfo, time.Time{}, "bbb")
	logger.RemoveLogger(logger1)
	logger.Log(logging.LevelInfo, time.Time{}, "ccc")

	if diff := cmp.Diff(logger1.Logs(), []string{"aaa", "bbb"}); diff != "" {
		t.Errorf("Messages mismatch for logger1 (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(logger2.Logs(), []string{"bbb", "ccc"}); diff != "" {
		t.Errorf("Messages mismatch for logger2 (-got +want):\n%s", diff)
	}
}
