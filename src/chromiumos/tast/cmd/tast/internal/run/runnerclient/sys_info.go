// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"path/filepath"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

// GetInitialSysInfo saves the initial state of the DUT's system information to cfg if
// requested and if it hasn't already been saved. This is called before testing.
// This updates state.InitialSysInfo, so calling twice won't work.
func GetInitialSysInfo(ctx context.Context, cfg *config.Config, cc *target.ConnCache) (*protocol.SysInfoState, error) {
	if !cfg.CollectSysInfo {
		return nil, nil
	}

	ctx, st := timing.Start(ctx, "initial_sys_info")
	defer st.End()
	logging.Debug(ctx, "Getting initial system state")

	conn, err := cc.Conn(ctx)
	if err != nil {
		return nil, err
	}

	var res jsonprotocol.RunnerGetSysInfoStateResult
	if err := runTestRunnerCommand(
		ctx,
		localRunnerCommand(cfg, conn.SSHConn()),
		&jsonprotocol.RunnerArgs{Mode: jsonprotocol.RunnerGetSysInfoStateMode},
		&res,
	); err != nil {
		return nil, err
	}

	for _, warn := range res.Warnings {
		logging.Info(ctx, "Error getting system info: ", warn)
	}
	return res.State.Proto(), nil
}

// collectSysInfo writes system information generated on the DUT during testing to the results dir if
// doing so was requested.
func collectSysInfo(ctx context.Context, cfg *config.Config, initialSysInfo *protocol.SysInfoState, cc *target.ConnCache) error {
	if !cfg.CollectSysInfo {
		return nil
	}

	ctx, st := timing.Start(ctx, "collect_sys_info")
	defer st.End()
	logging.Debug(ctx, "Collecting system information")

	conn, err := cc.Conn(ctx)
	if err != nil {
		return err
	}

	var res jsonprotocol.RunnerCollectSysInfoResult
	if err := runTestRunnerCommand(
		ctx,
		localRunnerCommand(cfg, conn.SSHConn()),
		&jsonprotocol.RunnerArgs{
			Mode:           jsonprotocol.RunnerCollectSysInfoMode,
			CollectSysInfo: &jsonprotocol.RunnerCollectSysInfoArgs{InitialState: *jsonprotocol.SysInfoStateFromProto(initialSysInfo)},
		},
		&res,
	); err != nil {
		return err
	}

	for _, warn := range res.Warnings {
		logging.Info(ctx, warn)
	}

	if len(res.LogDir) != 0 {
		if err := moveFromHost(ctx, cfg, conn.SSHConn(), res.LogDir, filepath.Join(cfg.ResDir, systemLogsDir)); err != nil {
			logging.Info(ctx, "Failed to copy system logs: ", err)
		}
	}
	if len(res.CrashDir) != 0 {
		if err := moveFromHost(ctx, cfg, conn.SSHConn(), res.CrashDir, filepath.Join(cfg.ResDir, crashesDir)); err != nil {
			logging.Info(ctx, "Failed to copy crashes: ", err)
		}
	}
	return nil
}
