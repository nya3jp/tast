// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import "strings"

// releaseInfo contains information parsed from /etc/lsb-release.
type releaseInfo struct {
	// board contains the board name as specified by CHROMEOS_RELEASE_BOARD, e.g. "cave".
	board string
	// builderPath contains the path to the built image as specified by
	// CHROMEOS_RELEASE_BUILDER_PATH, e.g. "cave-release/R65-10286.0.0".
	builderPath string
}

// getReleaseInfo parses data (typically the contents of /etc/lsb-release)
// and returns information about the system image.
func getReleaseInfo(data string) *releaseInfo {
	info := releaseInfo{}
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "CHROMEOS_RELEASE_BOARD":
			info.board = parts[1]
		case "CHROMEOS_RELEASE_BUILDER_PATH":
			info.builderPath = parts[1]
		}
	}
	return &info
}
