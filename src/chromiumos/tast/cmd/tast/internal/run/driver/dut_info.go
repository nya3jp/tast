// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

// GetDUTInfo retrieves various DUT information needed for test execution.
func (d *Driver) GetDUTInfo(ctx context.Context) (*protocol.DUTInfo, error) {
	if !d.cfg.CheckTestDeps() {
		return nil, nil
	}

	client := d.localRunnerClient()
	if client == nil {
		logging.Info(ctx, "Dont have access to DUT. Returning nil DUTInfo")
		return nil, nil
	}

	ctx, st := timing.Start(ctx, "get_dut_info")
	defer st.End()
	logging.Debug(ctx, "Getting DUT info")

	req := &protocol.GetDUTInfoRequest{ExtraUseFlags: d.cfg.ExtraUSEFlags()}
	res, err := client.GetDUTInfo(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve DUT info")
	}
	return res.GetDutInfo(), nil
}
