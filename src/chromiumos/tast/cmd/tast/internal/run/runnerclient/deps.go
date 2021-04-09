// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/timing"
)

// deviceConfigFile is a file name containing the dump of obtained device.Config of the DUT,
// which is directly under ResDir.
const deviceConfigFile = "device-config.txt"

// GetDUTInfo executes local_test_runner on the DUT to get a list of DUT info.
// The info is used to check tests' dependencies.
// This updates state.SoftwareFeatures, thus calling this twice won't work.
func GetDUTInfo(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) error {
	if !cfg.CheckTestDeps {
		return nil
	}
	if state.SoftwareFeatures != nil {
		return errors.New("GetDUTInfo is already called")
	}

	ctx, st := timing.Start(ctx, "get_dut_info")
	defer st.End()
	cfg.Logger.Debug("Getting DUT info")

	conn, err := cc.Conn(ctx)
	if err != nil {
		return err
	}

	var res jsonprotocol.RunnerGetDUTInfoResult
	if err := runTestRunnerCommand(
		localRunnerCommand(ctx, cfg, conn.SSHConn()),
		&jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerGetDUTInfoMode,
			GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
				ExtraUSEFlags:       cfg.ExtraUSEFlags,
				RequestDeviceConfig: true,
			},
		},
		&res,
	); err != nil {
		return err
	}

	// If the software feature is empty, then the DUT doesn't know about its features
	// (e.g. because it's a non-test image and doesn't have a listing of relevant USE flags).
	if res.SoftwareFeatures == nil {
		cfg.Logger.Debug("No software features reported by DUT -- non-test image?")
		return errors.New("can't check test deps; no software features reported by DUT")
	}

	for _, warn := range res.Warnings {
		cfg.Logger.Log(warn)
	}

	state.OSVersion = res.OSVersion
	state.DefaultBuildArtifactsURL = res.DefaultBuildArtifactsURL

	cfg.Logger.Debug("Software features supported by DUT: ", strings.Join(res.SoftwareFeatures.Available, " "))
	if res.DeviceConfig != nil {
		cfg.Logger.Debug("Got DUT device.Config data; dumping to ", deviceConfigFile)
		if err := ioutil.WriteFile(filepath.Join(cfg.ResDir, deviceConfigFile), []byte(proto.MarshalTextString(res.DeviceConfig)), 0644); err != nil {
			cfg.Logger.Debugf("Failed to dump %s: %v", deviceConfigFile, err)
		}
		state.DeviceConfig = res.DeviceConfig
		state.HardwareFeatures = res.HardwareFeatures
	}
	state.SoftwareFeatures = res.SoftwareFeatures
	return nil
}

// featureArgsFromConfig returns feature arguments based on the configuration parameter.
func featureArgsFromConfig(cfg *config.Config, state *config.State) *jsonprotocol.FeatureArgs {
	args := jsonprotocol.FeatureArgs{
		CheckDeps:        cfg.CheckTestDeps,
		TestVars:         cfg.TestVars,
		MaybeMissingVars: cfg.MaybeMissingVars,
	}
	if state.SoftwareFeatures != nil {
		args.AvailableSoftwareFeatures = state.SoftwareFeatures.Available
		args.UnavailableSoftwareFeatures = state.SoftwareFeatures.Unavailable
		args.DeviceConfig.Proto = state.DeviceConfig
		args.HardwareFeatures.Proto = state.HardwareFeatures
	}
	return &args
}
