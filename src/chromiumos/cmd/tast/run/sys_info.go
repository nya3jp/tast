// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"path/filepath"

	"chromiumos/tast/runner"
	"chromiumos/tast/timing"
)

// getInitialSysInfo saves the initial state of the DUT's system information to cfg if
// requested and if it hasn't already been saved. This is called before testing.
func getInitialSysInfo(ctx context.Context, cfg *Config) error {
	if !cfg.collectSysInfo || cfg.initialSysInfo != nil {
		return nil
	}

	defer timing.Start(ctx, "initial_sys_info").End()
	cfg.Logger.Debug("Getting initial system state")

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return err
	}
	handle, err := startLocalRunner(ctx, cfg, hst, &runner.Args{Mode: runner.GetSysInfoStateMode})
	if err != nil {
		return err
	}
	defer handle.Close(ctx)

	var res runner.GetSysInfoStateResult
	if err = readLocalRunnerOutput(ctx, handle, &res); err != nil {
		return err
	}
	for _, warn := range res.Warnings {
		cfg.Logger.Log("Error getting system info: ", warn)
	}
	cfg.initialSysInfo = &res.State
	return nil
}

// collectSysInfo writes system information generated on the DUT during testing to the results dir if
// doing so was requested. This is called after testing and relies on the state saved by
// getInitialSysInfo.
func collectSysInfo(ctx context.Context, cfg *Config) error {
	if !cfg.collectSysInfo || cfg.initialSysInfo == nil {
		return nil
	}

	defer timing.Start(ctx, "collect_sys_info").End()
	cfg.Logger.Debug("Collecting system information")

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return err
	}
	args := runner.Args{
		Mode:               runner.CollectSysInfoMode,
		CollectSysInfoArgs: runner.CollectSysInfoArgs{InitialState: *cfg.initialSysInfo},
	}
	handle, err := startLocalRunner(ctx, cfg, hst, &args)
	if err != nil {
		return err
	}
	defer handle.Close(ctx)

	var res runner.CollectSysInfoResult
	if err = readLocalRunnerOutput(ctx, handle, &res); err != nil {
		return err
	}

	for _, warn := range res.Warnings {
		cfg.Logger.Log(warn)
	}

	if len(res.LogDir) != 0 {
		cfg.Logger.Status("Copying system logs")
		if err := moveFromHost(ctx, cfg, hst, res.LogDir, filepath.Join(cfg.ResDir, systemLogsDir)); err != nil {
			cfg.Logger.Log("Failed to copy system logs: ", err)
		}
	}
	if len(res.CrashDir) != 0 {
		cfg.Logger.Status("Copying crashes")
		if err := moveFromHost(ctx, cfg, hst, res.CrashDir, filepath.Join(cfg.ResDir, crashesDir)); err != nil {
			cfg.Logger.Log("Failed to copy crashes: ", err)
		}
	}
	return nil
}
