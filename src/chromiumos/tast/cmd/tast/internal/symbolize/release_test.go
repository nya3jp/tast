// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"testing"

	"chromiumos/tast/cmd/tast/internal/symbolize/breakpad"
)

func TestGetReleaseInfo(t *testing.T) {
	for _, tc := range []struct {
		lsbRelease                        string
		annotations                       map[string]string
		isError                           bool
		board, builderPath, lacrosVersion string
	}{
		{
			board:       "",
			builderPath: "",
		},
		{
			lsbRelease:  "CHROMEOS_RELEASE_BOARD=foo",
			board:       "foo",
			builderPath: "",
		},
		{
			lsbRelease:  "CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar",
			board:       "foo",
			builderPath: "bar",
		},
		{
			lsbRelease:  "CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\n",
			board:       "foo",
			builderPath: "bar/baz",
		},
		{
			lsbRelease:  "A=a\nCHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\nB=b\n",
			board:       "foo",
			builderPath: "bar/baz",
		},
		{
			annotations: map[string]string{
				"channel": "stable",
				"plat":    "Linux",
			},
			board:       "",
			builderPath: "",
		},
		{
			annotations: map[string]string{
				"channel":        "stable",
				"plat":           "Linux",
				"chromeos-board": "samus",
			},
			board:       "samus",
			builderPath: "",
		},
		{
			annotations: map[string]string{
				"channel":               "stable",
				"plat":                  "Linux",
				"chromeos-builder-path": "samus-release/R93-14040.0.0",
			},
			board:       "",
			builderPath: "samus-release/R93-14040.0.0",
		},
		{
			annotations: map[string]string{
				"channel":               "unknown",
				"chromeos-board":        "nautilus",
				"chromeos-builder-path": "nautilus-release/R96-14250.0.0",
				"prod":                  "Chrome_Lacros",
				"ver":                   "95.0.4637.0",
			},
			board:         "nautilus",
			builderPath:   "nautilus-release/R96-14250.0.0",
			lacrosVersion: "95.0.4637.0",
		},
		{
			annotations: map[string]string{
				"channel":               "unknown",
				"chromeos-board":        "nautilus",
				"chromeos-builder-path": "nautilus-release/R96-14250.0.0",
				"prod":                  "Chrome_Lacros",
			},
			isError: true,
		},
	} {
		data := &breakpad.MinidumpReleaseInfo{
			EtcLsbRelease:       tc.lsbRelease,
			CrashpadAnnotations: tc.annotations,
		}
		info, err := getReleaseInfo(data)
		if tc.isError {
			if err == nil {
				t.Errorf("getReleaseInfo(%q) was successful, but wanted error", data)
			}
			continue
		}
		if err != nil {
			t.Errorf("getReleaseInfo(%q) returned an error: %v", data, err)
			continue
		}
		if info.board != tc.board {
			t.Errorf("getReleaseInfo(%q).board = %q; want %q", data, info.board, tc.board)
		}
		if info.builderPath != tc.builderPath {
			t.Errorf("getReleaseInfo(%q).builderPath = %q; want %q", data, info.builderPath, tc.builderPath)
		}
		if info.lacrosVersion != tc.lacrosVersion {
			t.Errorf("getReleaseInfo(%q).lacrosVersion = %q; want %q", data, info.lacrosVersion, tc.lacrosVersion)
		}
	}
}
