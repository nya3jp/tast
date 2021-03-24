// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"errors"
	"path/filepath"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/timing"
)

// GetInitialSysInfo saves the initial state of the DUT's system information to cfg if
// requested and if it hasn't already been saved. This is called before testing.
// This updates state.InitialSysInfo, so calling twice won't work.
func GetInitialSysInfo(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) error {
	if !cfg.CollectSysInfo {
		return nil
	}
	if state.InitialSysInfo != nil {
		return errors.New("GetInitialSysInfo is already called")
	}

	ctx, st := timing.Start(ctx, "initial_sys_info")
	defer st.End()
	cfg.Logger.Debug("Getting initial system state")

	conn, err := cc.Conn(ctx)
	if err != nil {
		return err
	}

	var res runner.GetSysInfoStateResult
	if err := runTestRunnerCommand(
		localRunnerCommand(ctx, cfg, conn.SSHConn()),
		&runner.Args{Mode: runner.GetSysInfoStateMode},
		&res,
	); err != nil {
		return err
	}

	for _, warn := range res.Warnings {
		cfg.Logger.Log("Error getting system info: ", warn)
	}
	state.InitialSysInfo = &res.State
	return nil
}

// collectSysInfo writes system information generated on the DUT during testing to the results dir if
// doing so was requested. This is called after testing and relies on the state saved by
// GetInitialSysInfo.
func collectSysInfo(ctx context.Context, cfg *config.Config, state *config.State) error {
	if !cfg.CollectSysInfo || state.InitialSysInfo == nil {
		return nil
	}

	ctx, st := timing.Start(ctx, "collect_sys_info")
	defer st.End()
	cfg.Logger.Debug("Collecting system information")

	// We have to create a new connection cache here since one created in
	// Run is not available to WriteResults.
	cc := target.NewConnCache(cfg)
	defer cc.Close(ctx)

	conn, err := cc.Conn(ctx)
	if err != nil {
		return err
	}

	var res runner.CollectSysInfoResult
	if err := runTestRunnerCommand(
		localRunnerCommand(ctx, cfg, conn.SSHConn()),
		&runner.Args{
			Mode:           runner.CollectSysInfoMode,
			CollectSysInfo: &runner.CollectSysInfoArgs{InitialState: *state.InitialSysInfo},
		},
		&res,
	); err != nil {
		return err
	}

	for _, warn := range res.Warnings {
		cfg.Logger.Log(warn)
	}

	if len(res.LogDir) != 0 {
		if err := moveFromHost(ctx, cfg, conn.SSHConn(), res.LogDir, filepath.Join(cfg.ResDir, systemLogsDir)); err != nil {
			cfg.Logger.Log("Failed to copy system logs: ", err)
		}
	}
	if len(res.CrashDir) != 0 {
		if err := moveFromHost(ctx, cfg, conn.SSHConn(), res.CrashDir, filepath.Join(cfg.ResDir, crashesDir)); err != nil {
			cfg.Logger.Log("Failed to copy crashes: ", err)
		}
	}
	return nil
}
