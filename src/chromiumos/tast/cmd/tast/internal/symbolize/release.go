// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"bytes"

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
}

// isEmpty is true if both board and builderPath are empty.
func (ri *releaseInfo) isEmpty() bool {
	return ri.board == "" && ri.builderPath == ""
}

// newEmptyReleaseInfo returns a releaseInfo with empty components.
func newEmptyReleaseInfo() *releaseInfo {
	return &releaseInfo{"", ""}
}

// getReleaseInfo parses data (the contents of /etc/lsb-release or Crashpad annotations)
// and returns information about the system image.
func getReleaseInfo(data *breakpad.MinidumpReleaseInfo) *releaseInfo {
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
	}
	return &info
}
