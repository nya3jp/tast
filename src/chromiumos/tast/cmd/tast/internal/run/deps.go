// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"

	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/timing"
)

// deviceConfigFile is a file name containing the dump of obtained device.Config of the DUT,
// which is directly under ResDir.
const deviceConfigFile = "device-config.txt"

// getDUTInfo executes local_test_runner on the DUT to get a list of DUT info.
// The info is used to check tests' dependencies.
// This updates state.softwareFeatures, thus calling this twice won't work.
func getDUTInfo(ctx context.Context, cfg *Config, state *State) error {
	if !cfg.checkTestDeps {
		return nil
	}
	if state.softwareFeatures != nil {
		return errors.New("getDUTInfo is already called")
	}

	ctx, st := timing.Start(ctx, "get_dut_info")
	defer st.End()
	cfg.Logger.Debug("Getting DUT info")

	hst, err := connectToTarget(ctx, cfg, state)
	if err != nil {
		return err
	}

	var res runner.GetDUTInfoResult
	if err := runTestRunnerCommand(
		localRunnerCommand(ctx, cfg, hst),
		&runner.Args{
			Mode: runner.GetDUTInfoMode,
			GetDUTInfo: &runner.GetDUTInfoArgs{
				ExtraUSEFlags:       cfg.extraUSEFlags,
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

	state.osVersion = res.OSVersion

	cfg.Logger.Debug("Software features supported by DUT: ", strings.Join(res.SoftwareFeatures.Available, " "))
	if res.DeviceConfig != nil {
		cfg.Logger.Debug("Got DUT device.Config data; dumping to ", deviceConfigFile)
		if err := ioutil.WriteFile(filepath.Join(cfg.ResDir, deviceConfigFile), []byte(proto.MarshalTextString(res.DeviceConfig)), 0644); err != nil {
			cfg.Logger.Debugf("Failed to dump %s: %v", deviceConfigFile, err)
		}
		state.deviceConfig = res.DeviceConfig
		state.hardwareFeatures = res.HardwareFeatures
	}
	state.softwareFeatures = res.SoftwareFeatures
	return nil
}

// featureArgsFromConfig returns feature arguments based on the configuration parameter.
func featureArgsFromConfig(cfg *Config, state *State) *bundle.FeatureArgs {
	args := bundle.FeatureArgs{
		CheckSoftwareDeps: cfg.checkTestDeps,
		TestVars:          cfg.testVars,
	}
	if state.softwareFeatures != nil {
		args.AvailableSoftwareFeatures = state.softwareFeatures.Available
		args.UnavailableSoftwareFeatures = state.softwareFeatures.Unavailable
		args.DeviceConfig.Proto = state.deviceConfig
		args.HardwareFeatures.Proto = state.hardwareFeatures
	}
	return &args
}
