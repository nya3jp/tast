// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"io"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/crosbundle"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// handleGetDUTInfo handles a RunnerGetDUTInfoMode request from args
// and JSON-marshals a RunnerGetDUTInfoResult struct to w.
func handleGetDUTInfo(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, w io.Writer) error {
	logger := newArrayLogger()
	ctx = logging.AttachLogger(ctx, logger)

	softwareFeatures, err := crosbundle.DetectSoftwareFeatures(
		ctx, scfg.SoftwareFeatureDefinitions, scfg.USEFlagsFile, scfg.LSBReleaseFile, args.GetDUTInfo.ExtraUSEFlags, scfg.AutotestCapabilityDir)
	if err != nil {
		return err
	}

	var hardwareFeatures *protocol.HardwareFeatures
	if args.GetDUTInfo.RequestDeviceConfig {
		var err error
		hardwareFeatures, err = crosbundle.DetectHardwareFeatures(ctx)
		if err != nil {
			return err
		}
	}

	dutInfo := &protocol.DUTInfo{
		Features: &protocol.DUTFeatures{
			Software: softwareFeatures,
			Hardware: hardwareFeatures,
		},
		OsVersion:                scfg.OSVersion,
		DefaultBuildArtifactsUrl: scfg.DefaultBuildArtifactsURL,
	}
	res := &protocol.GetDUTInfoResponse{DutInfo: dutInfo}

	jres := jsonprotocol.RunnerGetDUTInfoResultFromProto(res)
	jres.Warnings = logger.Logs()

	if err := json.NewEncoder(w).Encode(&res); err != nil {
		return command.NewStatusErrorf(statusError, "failed to serialize into JSON: %v", err)
	}
	return nil
}
