// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing_test

import (
	"context"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/testing"
)

func TestContextLogger(t *gotesting.T) {
	ctx := context.Background()

	if _, ok := testing.ContextLogger(ctx); ok {
		t.Errorf("Expected logger to not be available from background context")
	}

	logger := loggingtest.NewLogger(t, logging.LevelInfo)
	ctx = logging.AttachLogger(ctx, logger)

	logger2, ok := testing.ContextLogger(ctx)
	if !ok {
		t.Errorf("Expected logger to be available")
	}

	const testLog = "foo"
	logger2.Print(testLog)
	if diff := cmp.Diff(logger.Logs(), []string{testLog}); diff != "" {
		t.Errorf("Log mismatch (-got +want):\n%s", diff)
	}
}
