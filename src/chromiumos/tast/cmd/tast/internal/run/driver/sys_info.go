// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"
	"path/filepath"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

const (
	// SystemLogsDir is a result subdirectory where system logs are saved
	// by CollectSysInfo.
	SystemLogsDir = "system_logs"

	// CrashesDir is a result subdirectory where crash dumps are saved by
	// CollectSysInfo.
	CrashesDir = "crashes"
)

// GetSysInfoState collects the sysinfo state of the DUT.
func (d *Driver) GetSysInfoState(ctx context.Context) (*protocol.SysInfoState, error) {
	if !d.cfg.CollectSysInfo() {
		return nil, nil
	}

	ctx, st := timing.Start(ctx, "get_sys_info_state")
	defer st.End()
	logging.Debug(ctx, "Getting initial system state")

	req := &protocol.GetSysInfoStateRequest{}
	res, err := d.localRunnerClient().GetSysInfoState(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get system info state")
	}
	return res.GetState(), nil
}

// CollectSysInfo collects the sysinfo, considering diff from the given initial
// sysinfo state.
func (d *Driver) CollectSysInfo(ctx context.Context, initialSysInfo *protocol.SysInfoState) (retErr error) {
	if !d.cfg.CollectSysInfo() {
		return nil
	}

	ctx, st := timing.Start(ctx, "collect_sys_info")
	defer st.End()
	logging.Debug(ctx, "Collecting system information")

	req := &protocol.CollectSysInfoRequest{
		InitialState: initialSysInfo,
	}
	res, err := d.localRunnerClient().CollectSysInfo(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed to collect system info")
	}

	if logDir := res.GetLogDir(); logDir != "" {
		if err := linuxssh.GetAndDeleteFile(ctx, d.SSHConn(), logDir, filepath.Join(d.cfg.ResDir(), SystemLogsDir), linuxssh.PreserveSymlinks); err != nil {
			return errors.Wrap(err, "failed to copy system logs")
		}
	}
	if crashDir := res.GetCrashDir(); crashDir != "" {
		if err := linuxssh.GetAndDeleteFile(ctx, d.SSHConn(), crashDir, filepath.Join(d.cfg.ResDir(), CrashesDir), linuxssh.PreserveSymlinks); err != nil {
			return errors.Wrap(err, "failed to copy crashes")
		}
	}
	return nil
}
