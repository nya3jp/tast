// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/internal/logging"
)

// StreamFile stream a file from the source file at target DUT to a destination
// file at the host.
func (d *Driver) StreamFile(ctx context.Context, src, dest string) error {
	nextOffset := int64(-1)
	for {
		if d.cc == nil || d.cc.Conn() == nil {
			// The current driver is no longer valid.
			return nil
		}
		client := d.localRunnerClient()
		if client == nil {
			logging.Infof(ctx, "Do not have access to DUT so %v will not be streamed", src)
			return nil
		}
		logging.Debugf(ctx, "Streaming the file %v from the DUT to %v", src, dest)
		if offset, err := client.StreamFile(ctx, src, dest, nextOffset); err != nil {
			if ctx.Err() != nil {
				// Context has an error. It can be due to cancellation or end of
				// the operation. There will other log to explain this situation.
				// Do not need to log in this siuation.
				return nil
			}
			logging.Infof(ctx, "Warning: failed to stream %s from DUT to %s: %v", src, dest, err)
			if strings.Contains(err.Error(), "does not exist on the DUT") {
				logging.Infof(ctx, "Fail %s does not exist; will not retry streaming", src)
				return nil
			}
			nextOffset = offset
		}
		// If the file exist on the server, keep trying.
		// Sleep for 30 second before retrying.
		time.Sleep(time.Second * 30)
		if err := d.ReconnectIfNeeded(ctx); err != nil {
			logging.Infof(ctx, "Failed to reconnect to DUT to stream file %v", err)
			return nil
		}
	}
}
