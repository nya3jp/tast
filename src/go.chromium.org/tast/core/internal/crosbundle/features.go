// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"context"
	"fmt"
	"os"

	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/lsbrelease"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

// GetDUTInfo implements the GetDUTInfo RPC method for ChromeOS.
func GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	var osVersion, defaultBuildArtifactsURL string

	if _, err := os.Stat(lsbrelease.Path); err == nil {
		kvs, err := lsbrelease.Load()
		if err != nil {
			return nil, err
		}

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
	} else {
		osVersion = "NotAvail"
	}

	var softwareFeatures *frameworkprotocol.SoftwareFeatures
	var hardwareFeatures *frameworkprotocol.HardwareFeatures

	if req.GetFeatures() {
		var err error
		softwareFeatures, err = detectSoftwareFeatures(ctx, req.GetExtraUseFlags())
		if err != nil {
			return nil, err
		}
		hardwareFeatures, err = detectHardwareFeatures(ctx)
		if err != nil {
			return nil, err
		}
	}

	return &protocol.GetDUTInfoResponse{
		DutInfo: &protocol.DUTInfo{
			Features: &frameworkprotocol.DUTFeatures{
				Software: softwareFeatures,
				Hardware: hardwareFeatures,
			},
			OsVersion:                osVersion,
			DefaultBuildArtifactsUrl: defaultBuildArtifactsURL,
		},
	}, nil
}
