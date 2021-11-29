// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"context"
	"fmt"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/lsbrelease"
)

// GetDUTInfo implements the GetDUTInfo RPC method for Chrome OS.
func GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	kvs, err := lsbrelease.Load()
	if err != nil {
		return nil, err
	}

	var osVersion, defaultBuildArtifactsURL string
	if bp := kvs[lsbrelease.BuilderPath]; bp != "" {
		osVersion = bp
		defaultBuildArtifactsURL = "gs://chromeos-image-archive/" + bp + "/"
	} else {
		// Sometimes CHROMEOS_RELEASE_BUILDER_PATH is not in /etc/lsb-release.
		// Make up the string in this case.
		board := kvs[lsbrelease.Board]
		version := kvs[lsbrelease.Version]
		milestone := kvs[lsbrelease.Milestone]
		buildType := kvs[lsbrelease.BuildType]
		osVersion = fmt.Sprintf("%vR%v-%v (%v)", board, milestone, version, buildType)
	}

	softwareFeatures, err := detectSoftwareFeatures(ctx, req.GetExtraUseFlags())
	if err != nil {
		return nil, err
	}

	hardwareFeatures, err := detectHardwareFeatures(ctx)
	if err != nil {
		return nil, err
	}

	return &protocol.GetDUTInfoResponse{
		DutInfo: &protocol.DUTInfo{
			Features: &protocol.DUTFeatures{
				Software: softwareFeatures,
				Hardware: hardwareFeatures,
			},
			OsVersion:                osVersion,
			DefaultBuildArtifactsUrl: defaultBuildArtifactsURL,
		},
	}, nil
}
