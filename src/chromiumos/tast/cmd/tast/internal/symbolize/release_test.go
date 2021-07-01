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
		lsbRelease         string
		annotations        map[string]string
		board, builderPath string
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
	} {
		data := &breakpad.MinidumpReleaseInfo{
			EtcLsbRelease:       tc.lsbRelease,
			CrashpadAnnotations: tc.annotations,
		}
		info := getReleaseInfo(data)
		if info.board != tc.board {
			t.Errorf("getReleaseInfo(%q).board = %q; want %q", data, info.board, tc.board)
		}
		if info.builderPath != tc.builderPath {
			t.Errorf("getReleaseInfo(%q).builderPath = %q; want %q", data, info.builderPath, tc.builderPath)
		}
	}
}
