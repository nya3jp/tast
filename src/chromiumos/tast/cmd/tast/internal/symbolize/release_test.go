// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"testing"
)

func TestGetReleaseInfo(t *testing.T) {
	for _, tc := range []struct{ data, board, builderPath string }{
		{"", "", ""},
		{"CHROMEOS_RELEASE_BOARD=foo", "foo", ""},
		{"CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar", "foo", "bar"},
		{"CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\n", "foo", "bar/baz"},
		{"CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\n", "foo", "bar/baz"},
		{"A=a\nCHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\nB=b\n", "foo", "bar/baz"},
	} {
		info := getReleaseInfo(tc.data)
		if info.board != tc.board {
			t.Errorf("getReleaseInfo(%q).board = %q; want %q", tc.data, info.board, tc.board)
		}
		if info.builderPath != tc.builderPath {
			t.Errorf("getReleaseInfo(%q).builderPath = %q; want %q", tc.data, info.builderPath, tc.builderPath)
		}
	}
}
