// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if err := Sleep(ctx, time.Millisecond); err != nil {
		t.Errorf("Sleep(%v) failed: %v", time.Millisecond, err)
	}
}

func TestSleepContextExpires(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if err := Sleep(ctx, time.Hour); err == nil {
		t.Errorf("Sleep(%v) succeeded", time.Hour)
	}
}
