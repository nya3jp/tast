// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/logging/loggingtest"
)

func TestLogging(t *testing.T) {
	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	ctx := logging.AttachLogger(context.Background(), logger)
	logging.Info(ctx, "a", "aa")
	logging.Infof(ctx, "b%sb", "b")
	logging.Debug(ctx, "c", "cc")
	logging.Debugf(ctx, "d%sd", "d")
	if diff := cmp.Diff(logger.Logs(), []string{"aaa", "bbb", "ccc", "ddd"}); diff != "" {
		t.Errorf("Messages mismatch (-got +want):\n%s", diff)
	}
}

func TestLogging_Level(t *testing.T) {
	logger := loggingtest.NewLogger(t, logging.LevelInfo)
	ctx := logging.AttachLogger(context.Background(), logger)
	logging.Info(ctx, "a", "aa")
	logging.Infof(ctx, "b%sb", "b")
	logging.Debug(ctx, "c", "cc")
	logging.Debugf(ctx, "d%sd", "d")
	if diff := cmp.Diff(logger.Logs(), []string{"aaa", "bbb"}); diff != "" {
		t.Errorf("Messages mismatch (-got +want):\n%s", diff)
	}
}

func TestLogging_NoLogger(t *testing.T) {
	ctx := context.Background()
	logging.Info(ctx, "a", "aa")
	logging.Infof(ctx, "b%sb", "b")
	logging.Debug(ctx, "c", "cc")
	logging.Debugf(ctx, "d%sd", "d")
}

func TestLogging_Propagate(t *testing.T) {
	logger1 := loggingtest.NewLogger(t, logging.LevelDebug)
	logger2 := loggingtest.NewLogger(t, logging.LevelInfo)
	logger3 := loggingtest.NewLogger(t, logging.LevelDebug)
	logger4 := loggingtest.NewLogger(t, logging.LevelInfo)

	ctx := context.Background()
	ctx = logging.AttachLogger(ctx, logger1)
	ctx = logging.AttachLogger(ctx, logger2)
	ctx = logging.AttachLoggerNoPropagation(ctx, logger3)
	ctx = logging.AttachLogger(ctx, logger4)

	logging.Info(ctx, "info")
	logging.Debug(ctx, "debug")

	for _, tc := range []struct {
		name   string
		logger *loggingtest.Logger
		want   []string
	}{
		{"logger1", logger1, nil},
		{"logger2", logger2, nil},
		{"logger3", logger3, []string{"info", "debug"}},
		{"logger4", logger4, []string{"info"}},
	} {
		if diff := cmp.Diff(tc.logger.Logs(), tc.want); diff != "" {
			t.Errorf("%s: messages mismatch (-got +want):\n%s", tc.name, diff)
		}
	}
}

func TestLogging_Branch(t *testing.T) {
	logger1 := loggingtest.NewLogger(t, logging.LevelInfo)
	logger2 := loggingtest.NewLogger(t, logging.LevelInfo)
	logger3 := loggingtest.NewLogger(t, logging.LevelInfo)

	ctx1 := logging.AttachLogger(context.Background(), logger1)
	ctx2 := logging.AttachLogger(ctx1, logger2)
	ctx3 := logging.AttachLogger(ctx1, logger3)

	logging.Info(ctx1, "aaa")
	logging.Info(ctx2, "bbb")
	logging.Info(ctx3, "ccc")

	for _, tc := range []struct {
		name   string
		logger *loggingtest.Logger
		want   []string
	}{
		{"logger1", logger1, []string{"aaa", "bbb", "ccc"}},
		{"logger2", logger2, []string{"bbb"}},
		{"logger3", logger3, []string{"ccc"}},
	} {
		if diff := cmp.Diff(tc.logger.Logs(), tc.want); diff != "" {
			t.Errorf("%s: messages mismatch (-got +want):\n%s", tc.name, diff)
		}
	}
}

func TestHasLogger(t *testing.T) {
	ctx := context.Background()
	if logging.HasLogger(ctx) {
		t.Error("HasLogger = true for a background context")
	}

	ctx = logging.AttachLogger(ctx, logging.NewMultiLogger())
	if !logging.HasLogger(ctx) {
		t.Error("HasLogger = false for a context with a logger attached")
	}
}

func TestSetLogPrefix(t *testing.T) {
	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	ctx := logging.AttachLogger(context.Background(), logger)
	ctx = logging.SetLogPrefix(ctx, "[Phase A setup] ")

	logging.Info(ctx, "a", "aa")
	logging.Infof(ctx, "b%sb", "b")
	logging.Debug(ctx, "c", "cc")
	logging.Debugf(ctx, "d%sd", "d")

	ctx = logging.UnsetLogPrefix(ctx)

	logging.Info(ctx, "a", "aa")
	logging.Infof(ctx, "b%sb", "b")
	logging.Debug(ctx, "c", "cc")
	logging.Debugf(ctx, "d%sd", "d")

	if diff := cmp.Diff(logger.Logs(), []string{"[Phase A setup] aaa", "[Phase A setup] bbb", "[Phase A setup] ccc", "[Phase A setup] ddd", "aaa", "bbb", "ccc", "ddd"}); diff != "" {
		t.Errorf("Messages mismatch (-got +want):\n%s", diff)
	}
}

func TestLoggingInvalidUTF8(t *testing.T) {
	const invalidUTF8 = "\xed"
	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	ctx := logging.AttachLogger(context.Background(), logger)
	logging.Info(ctx, "a", "aa", invalidUTF8)
	logging.Infof(ctx, "b%sb%s", "b", invalidUTF8)
	logging.Debug(ctx, "c", "cc", invalidUTF8)
	logging.Debugf(ctx, "d%sd%s", "d", invalidUTF8)
	for _, l := range logger.Logs() {
		if strings.Contains(l, invalidUTF8) {
			t.Errorf("Log message %q has invalid UTF-8 characters", l)
		}
	}
}
