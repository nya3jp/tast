// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"

	"github.com/golang/protobuf/proto"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/timing"
)

// dutInfoFile is a file name containing the dump of obtained DUTInfo message,
// which is directly under ResDir.
const dutInfoFile = "dut-info.txt"

// GetDUTInfo executes local_test_runner on the DUT to get a list of DUT info.
// The info is used to check tests' dependencies.
// This updates state.SoftwareFeatures, thus calling this twice won't work.
func GetDUTInfo(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) error {
	if !cfg.CheckTestDeps {
		return nil
	}
	if state.DUTInfo != nil {
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
		ctx,
		localRunnerCommand(cfg, conn.SSHConn()),
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
	// TODO(b/187793617): Remove this check once we migrate to gRPC protocol
	// and GetDUTInfo gets ability to return errors.
	if res.SoftwareFeatures == nil {
		cfg.Logger.Debug("No software features reported by DUT -- non-test image?")
		return errors.New("can't check test deps; no software features reported by DUT")
	}

	for _, warn := range res.Warnings {
		cfg.Logger.Log(warn)
	}

	info := res.Proto()

	if err := ioutil.WriteFile(filepath.Join(cfg.ResDir, dutInfoFile), []byte(proto.MarshalTextString(info)), 0644); err != nil {
		cfg.Logger.Debugf("Failed to dump DUTInfo: %v", err)
	}

	state.DUTInfo = info
	return nil
}

// featureArgsFromConfig returns feature arguments based on the configuration parameter.
func featureArgsFromConfig(cfg *config.Config, state *config.State) *jsonprotocol.FeatureArgs {
	f := state.DUTInfo.GetFeatures()
	return &jsonprotocol.FeatureArgs{
		CheckDeps:                   cfg.CheckTestDeps,
		TestVars:                    cfg.TestVars,
		MaybeMissingVars:            cfg.MaybeMissingVars,
		AvailableSoftwareFeatures:   f.GetSoftware().GetAvailable(),
		UnavailableSoftwareFeatures: f.GetSoftware().GetUnavailable(),
		DeviceConfig:                jsonprotocol.DeviceConfigJSON{Proto: f.GetHardware().GetDeprecatedDeviceConfig()},
		HardwareFeatures:            jsonprotocol.HardwareFeaturesJSON{Proto: f.GetHardware().GetHardwareFeatures()},
	}
}
