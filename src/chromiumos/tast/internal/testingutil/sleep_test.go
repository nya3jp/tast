// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testingutil_test

import (
	"context"
	"testing"
	"time"

	"chromiumos/tast/internal/testingutil"
)

func TestSleep(t *testing.T) {
	const timeout = 20 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	const sleep = time.Millisecond
	start := time.Now()
	if err := testingutil.Sleep(ctx, sleep); err != nil {
		t.Errorf("Sleep(%v, %v) failed: %v", timeout, sleep, err)
	}
	if d := time.Since(start); d >= timeout {
		t.Errorf("Sleep(%v, %v) slept %v, ignoring sleep duration", timeout, sleep, d)
	}
}

func TestSleepContextExpires(t *testing.T) {
	const timeout = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	const sleep = 20 * time.Second
	start := time.Now()
	if err := testingutil.Sleep(ctx, sleep); err == nil {
		t.Errorf("Sleep(%v, %v) returned no error", timeout, sleep)
	}
	if d := time.Since(start); d >= sleep {
		t.Errorf("Sleep(%v, %v) slept %v, ignoring ctx timeout", timeout, sleep, d)
	}
}
