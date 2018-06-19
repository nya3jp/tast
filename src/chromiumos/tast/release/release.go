// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package release provides utilities to parse /etc/lsb-release.
package release

import (
	"io/ioutil"
	"strings"
)

// Info contains information parsed from /etc/lsb-release.
type Info struct {
	// Board contains the board name as specified by CHROMEOS_RELEASE_BOARD, e.g. "cave".
	Board string
	// BuilderPath contains the path to the built image as specified by
	// CHROMEOS_RELEASE_BUILDER_PATH, e.g. "cave-release/R65-10286.0.0".
	BuilderPath string
}

// Parse parses data (typically the contents of /etc/lsb-release)
// and returns information about the system image.
func Parse(data string) *Info {
	info := Info{}
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "CHROMEOS_RELEASE_BOARD":
			info.Board = parts[1]
		case "CHROMEOS_RELEASE_BUILDER_PATH":
			info.BuilderPath = parts[1]
		}
	}
	return &info
}

// Load loads and parses /etc/lsb-release in the local filesystem.
func Load() (*Info, error) {
	data, err := ioutil.ReadFile("/etc/lsb-release")
	if err != nil {
		return nil, err
	}
	return Parse(string(data)), nil
}
