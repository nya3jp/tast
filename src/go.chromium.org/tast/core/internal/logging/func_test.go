// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/tast/core/internal/logging"
)

func TestFuncLogger(t *testing.T) {
	var gotLevels []logging.Level
	var gotTimes []time.Time
	var gotMsgs []string
	logger := logging.NewFuncLogger(func(level logging.Level, ts time.Time, msg string) {
		gotLevels = append(gotLevels, level)
		gotTimes = append(gotTimes, ts)
		gotMsgs = append(gotMsgs, msg)
	})
	logger.Log(logging.LevelDebug, time.UnixMilli(1), "foo")
	logger.Log(logging.LevelInfo, time.UnixMilli(2), "bar\nbaz\n")

	wantLevels := []logging.Level{logging.LevelDebug, logging.LevelInfo}
	if diff := cmp.Diff(gotLevels, wantLevels); diff != "" {
		t.Fatalf("Levels mismatch (-got +want):\n%s", diff)
	}
	wantTimes := []time.Time{time.UnixMilli(1), time.UnixMilli(2)}
	if diff := cmp.Diff(gotTimes, wantTimes); diff != "" {
		t.Fatalf("Messages mismatch (-got +want):\n%s", diff)
	}
	wantMsgs := []string{"foo", "bar\nbaz\n"}
	if diff := cmp.Diff(gotMsgs, wantMsgs); diff != "" {
		t.Fatalf("Messages mismatch (-got +want):\n%s", diff)
	}
}
