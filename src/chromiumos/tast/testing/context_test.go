// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	"testing"

	"chromiumos/tast/internal/logging"
)

func TestContextLogger(t *testing.T) {
	ctx := context.Background()

	if _, ok := ContextLogger(ctx); ok {
		t.Errorf("Expected logger to not be available from background context")
	}

	var logs []string
	sink := func(msg string) {
		logs = append(logs, msg)
	}
	ctx = logging.NewContext(ctx, sink)

	logger, ok := ContextLogger(ctx)
	if !ok {
		t.Errorf("Expected logger to be available")
	}

	const testLog = "foo"
	logger.Print(testLog)
	if exp := []string{testLog}; !reflect.DeepEqual(logs, exp) {
		t.Errorf("Print did not work as expected: got %v, want %v", logs, exp)
	}
}
