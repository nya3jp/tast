// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lsbrelease provides a parser of /etc/lsb-release.
//
// Usually Tast tests are not supposed to use information in /etc/lsb-release to
// change their behavior, so access to functions in this package is restricted
// unless explicitly permitted.
package lsbrelease

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"

	"chromiumos/tast/caller"
)

// Keys in /etc/lsb-release. See the following doc for details:
//  https://chromium.googlesource.com/chromiumos/docs/+/master/os_config.md#LSB
const (
	// Board is a key for a board name (e.g. "eve")
	Board = "CHROMEOS_RELEASE_BOARD"

	// BuilderPath is a key for a string identifying the builder run that
	// produced the Chrome OS image (e.g. "eve-release/R74-12345.0.0").
	BuilderPath = "CHROMEOS_RELEASE_BUILDER_PATH"

	// Milestone is a key for milestone number (e.g. "74")
	Milestone = "CHROMEOS_RELEASE_CHROME_MILESTONE"

	// Version is a key for Chrome OS version (e.g. "12345.0.0")
	Version = "CHROMEOS_RELEASE_VERSION"

	// ReleaseAppID is a key for the release Omaha app ID.
	ReleaseAppID = "CHROMEOS_RELEASE_APPID"

	// ARCSDKVersion is a key for the Android SDK Version of the current
	// ARC image installed on the DUT.
	ARCSDKVersion = "CHROMEOS_ARC_ANDROID_SDK_VERSION"
)

// allowedPkgs is the list of Go packages that can use this package.
var allowedPkgs = []string{
	"chromiumos/tast/cmd/tast/internal/symbolize",
	"chromiumos/tast/local/arc",              // For SDKVersion.
	"chromiumos/tast/local/bundles/cros/arc", // For Version.
	"chromiumos/tast/local/bundles/cros/crash/sender",
	"chromiumos/tast/local/bundles/cros/platform/updateserver",
	"chromiumos/tast/local/rialto",
	"chromiumos/tast/lsbrelease",
	"main", // for local_test_runner
}

// Load loads /etc/lsb-release and returns a parsed key-value map.
//
// Usually Tast tests are not supposed to use information in /etc/lsb-release to
// change their behavior, so access to this function is restricted unless
// explicitly permitted by allowedPkgs.
func Load() (map[string]string, error) {
	caller.Check(2, allowedPkgs)

	f, err := os.Open("/etc/lsb-release")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return Parse(f)
}

// lineRe matches a key-value line after surrounding whitespaces in /etc/lsb-release.
var lineRe = regexp.MustCompile(`^([A-Z0-9_]+)\s*=\s*(.*)$`)

// Parse parses a key-value text file in the /etc/lsb-release format, and returns
// a parsed key-value map.
//
// Usually Tast tests are not supposed to use information in /etc/lsb-release to
// change their behavior, so access to this function is restricted unless
// explicitly permitted by allowedPkgs.
func Parse(r io.Reader) (map[string]string, error) {
	caller.Check(2, allowedPkgs)

	// The format of /etc/lsb-release in Chrome OS is described in the following doc:
	// https://chromium.googlesource.com/chromiumos/docs/+/master/lsb-release.md
	kvs := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kvs[m[1]] = m[2]
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return kvs, nil
}
