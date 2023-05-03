// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testingutil

import (
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
)

// Sleep implements testing.Sleep.
func Sleep(ctx context.Context, d time.Duration) error {
	tm := time.NewTimer(d)
	defer tm.Stop()

	select {
	case <-tm.C:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "sleep interrupted")
	}
}
