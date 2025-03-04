// Copyright 2019 The ChromiumOS Authors
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

	"go.chromium.org/tast/core/caller"
)

// Path is to the path for lsb-release file on the device.
const Path = "/etc/lsb-release"

// Keys in /etc/lsb-release. See the following doc for details:
//
//	https://chromium.googlesource.com/chromiumos/docs/+/HEAD/os_config.md#LSB
const (
	// Board is a key for a board name (e.g. "eve")
	Board = "CHROMEOS_RELEASE_BOARD"

	// BuilderPath is a key for a string identifying the builder run that
	// produced the ChromeOS image (e.g. "eve-release/R74-12345.5.0").
	BuilderPath = "CHROMEOS_RELEASE_BUILDER_PATH"

	// Milestone is a key for milestone number (e.g. "74")
	Milestone = "CHROMEOS_RELEASE_CHROME_MILESTONE"

	// BuildNumber is a key for the major OS version number (e.g. "12345")
	BuildNumber = "CHROMEOS_RELEASE_BUILD_NUMBER"

	// BranchNumber is a key for the minor OS version number used by branch (e.g. "5")
	BranchNumber = "CHROMEOS_RELEASE_BRANCH_NUMBER"

	// PatchNumber is a key for the patch number (e.g. "0")
	PatchNumber = "CHROMEOS_RELEASE_PATCH_NUMBER"

	// Version is a key for ChromeOS version (e.g. "12345.5.0")
	Version = "CHROMEOS_RELEASE_VERSION"

	// ReleaseAppID is a key for the release Omaha app ID.
	ReleaseAppID = "CHROMEOS_RELEASE_APPID"

	// BoardAppID is a key for the board Omaha app ID.
	BoardAppID = "CHROMEOS_BOARD_APPID"

	// BuildType is a key for Chrome Release Build Type (e.g "Test Build - username")
	BuildType = "CHROMEOS_RELEASE_BUILD_TYPE"

	// ReleaseTrack is a key for the device's release track (e.g. "stable-channel")
	ReleaseTrack = "CHROMEOS_RELEASE_TRACK"

	// ReleaseDescription is a key for the device's release description (e.g. "12345.5.0 (Official Build) dev-channel eve test")
	ReleaseDescription = "CHROMEOS_RELEASE_DESCRIPTION"

	// ARCSDKVersion is a key for the Android SDK Version of the current
	// ARC image installed on the DUT.
	ARCSDKVersion = "CHROMEOS_ARC_ANDROID_SDK_VERSION"

	// ARCVersion is a key for the Android Version of the current ARC image
	// installed on the DUT
	ARCVersion = "CHROMEOS_ARC_VERSION"
)

// allowedPkgs is the list of Go packages that can use this package.
var allowedPkgs = []string{
	"go.chromium.org/tast/core/cmd/tast/internal/symbolize",
	"go.chromium.org/tast/core/internal/crosbundle",          // For software feature detection.
	"go.chromium.org/tast/core/internal/runner",              // For SoftwareDeps check.
	"go.chromium.org/tast-tests/cros/common/firmware/usb",    // For checking USB images
	"go.chromium.org/tast-tests/cros/local/arc",              // For SDKVersion.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc", // For Version.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/platform/updateserver",
	"go.chromium.org/tast-tests/cros/local/bundles/cros/autoupdate",   // For autoupdate and rollback tests.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/health",       // To confirm OS version can be parsed.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/osinstall",    // For Version.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/runtimeprobe", // For Version.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/system",       // For Version.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec",        // For cross-version login tests.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy",       // For autoupdate policy tests.
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice",
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare",
	"go.chromium.org/tast-tests/cros/local/crash",
	"go.chromium.org/tast-tests/cros/local/graphics/trace",
	"go.chromium.org/tast-tests/cros/local/graphics/expectations", // For per-board test expectations.
	"go.chromium.org/tast-tests/cros/local/screenshot",            // For Board.
	"go.chromium.org/tast-tests/cros/local/uidetection",           // Build and board for uidetection analytics.
	"go.chromium.org/tast/core/lsbrelease",
	"go.chromium.org/tast/core/lsbrelease_test",
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware",          // For finding firmware file.
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/omaha/params",      // To replicate update_engine behavior.
	"go.chromium.org/tast-tests/cros/remote/firmware",                       // For checking USB images
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters",             // For Board.
	"go.chromium.org/tast-tests/cros/remote/sysutil",                        // For Board.
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wifi",               // For Board.
	"go.chromium.org/tast-tests-private/crosint/local/bundles/crosint/apps", // For Board.
}

// Load loads /etc/lsb-release and returns a parsed key-value map.
//
// Usually Tast tests are not supposed to use information in /etc/lsb-release to
// change their behavior, so access to this function is restricted unless
// explicitly permitted by allowedPkgs.
func Load() (map[string]string, error) {
	caller.Check(2, allowedPkgs)
	return LoadFrom(Path)
}

// LoadFrom loads the LSB-release map from the given path, and returns
// a parsed key-value map.
func LoadFrom(path string) (map[string]string, error) {
	caller.Check(2, allowedPkgs)

	f, err := os.Open(path)
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

	// The format of /etc/lsb-release in ChromeOS is described in the following doc:
	// https://chromium.googlesource.com/chromiumos/docs/+/HEAD/lsb-release.md
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
