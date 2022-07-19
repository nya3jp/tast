// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/runner"
)

// StreamFile stream a file from the source file at target DUT to a destination
// file at the host.
// Return the location where the next read should start.
func (d *Driver) StreamFile(ctx context.Context, src, dest string) error {
	client := d.localRunnerClient()
	if client == nil {
		logging.Infof(ctx, "Dont have access to DUT so %v will not be streamed", src)
		return nil
	}

	logging.Debugf(ctx, "Streaming the file %v from the DUT to %v", src, dest)

	go func() {
		nextOffset := int64(-1)
		for {
			if offset, err := client.StreamFile(ctx, src, dest, nextOffset); err != nil {
				if ctx.Err() != nil {
					// Context has an error. It can be due to cancellation or end of
					// the operation. There will other log to explain this situation.
					// Do not need to log in this siuation.
					return
				}
				logging.Infof(ctx, "Failed to stream %s from DUT to %s: %v", src, dest, err)
				if errors.Is(err, runner.ErrFailedToReadFile) {
					return
				}
				// If the file exist on the server, keep trying.
				// Sleep for 30 second before retrying.
				time.Sleep(time.Second * 30)
				nextOffset = offset
			}
		}
	}()
	return nil
}
