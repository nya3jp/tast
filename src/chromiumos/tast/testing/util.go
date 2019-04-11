// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"time"

	"chromiumos/tast/errors"
)

// Sleep pauses the current goroutine for d or until ctx expires.
//
// Please consider using testing.Poll instead. Sleeping without polling for
// a condition is discouraged, since it makes tests flakier (when the sleep
// duration isn't long enough) or slower (when the duration is too long).
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
