// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"bytes"
	"errors"

	"chromiumos/tast/cmd/tast/internal/symbolize/breakpad"
	"chromiumos/tast/lsbrelease"
)

// releaseInfo contains information parsed from /etc/lsb-release.
type releaseInfo struct {
	// board contains the board name as specified by CHROMEOS_RELEASE_BOARD, e.g. "cave".
	board string
	// builderPath contains the path to the built image as specified by
	// CHROMEOS_RELEASE_BUILDER_PATH, e.g. "cave-release/R65-10286.0.0".
	builderPath string
	// lacrosVersion is non-empty on minidumps from Lacros Chrome and contains
	// the version as specified in the "ver" Crashpad annotation.
	lacrosVersion string
}

// hasBuildInfo is true if both board or builderPath are populated.
func (ri *releaseInfo) hasBuildInfo() bool {
	return ri.board != "" || ri.builderPath != ""
}

// getReleaseInfo parses data (the contents of /etc/lsb-release or Crashpad annotations)
// and returns information about the system image.
func getReleaseInfo(data *breakpad.MinidumpReleaseInfo) (*releaseInfo, error) {
	var info releaseInfo
	if data.EtcLsbRelease != "" {
		kvs, err := lsbrelease.Parse(bytes.NewBufferString(data.EtcLsbRelease))
		if err == nil {
			info.board = kvs[lsbrelease.Board]
			info.builderPath = kvs[lsbrelease.BuilderPath]
		}
	} else {
		info.board = data.CrashpadAnnotations["chromeos-board"]
		info.builderPath = data.CrashpadAnnotations["chromeos-builder-path"]
		if data.CrashpadAnnotations["prod"] == "Chrome_Lacros" {
			info.lacrosVersion = data.CrashpadAnnotations["ver"]
			if info.lacrosVersion == "" {
				return nil, errors.New("Lacros Chrome does not specify the version")
			}
		}
	}
	return &info, nil
}
