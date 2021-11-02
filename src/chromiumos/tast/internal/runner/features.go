// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"encoding/json"
	"io"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/crosbundle"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
)

// handleGetDUTInfo handles a RunnerGetDUTInfoMode request from args
// and JSON-marshals a RunnerGetDUTInfoResult struct to w.
func handleGetDUTInfo(args *jsonprotocol.RunnerArgs, scfg *StaticConfig, w io.Writer) error {
	features, warnings, err := crosbundle.DetectSoftwareFeatures(
		scfg.SoftwareFeatureDefinitions, scfg.USEFlagsFile, scfg.LSBReleaseFile, args.GetDUTInfo.ExtraUSEFlags, scfg.AutotestCapabilityDir)
	if err != nil {
		return err
	}

	var dc *protocol.DeprecatedDeviceConfig
	var hwFeatures *configpb.HardwareFeatures
	if args.GetDUTInfo.RequestDeviceConfig {
		var ws []string
		dc, hwFeatures, ws = crosbundle.DetectHardwareFeatures()
		warnings = append(warnings, ws...)
	}

	res := jsonprotocol.RunnerGetDUTInfoResult{
		SoftwareFeatures:         features,
		DeviceConfig:             dc,
		HardwareFeatures:         hwFeatures,
		OSVersion:                scfg.OSVersion,
		DefaultBuildArtifactsURL: scfg.DefaultBuildArtifactsURL,
		Warnings:                 warnings,
	}
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		return command.NewStatusErrorf(statusError, "failed to serialize into JSON: %v", err)
	}
	return nil
}
