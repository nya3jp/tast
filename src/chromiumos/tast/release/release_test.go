// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package release

import (
	"testing"
)

func TestParseRelease(t *testing.T) {
	for _, tc := range []struct{ data, board, builderPath string }{
		{"", "", ""},
		{"CHROMEOS_RELEASE_BOARD=foo", "foo", ""},
		{"CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar", "foo", "bar"},
		{"CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\n", "foo", "bar/baz"},
		{"CHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\n", "foo", "bar/baz"},
		{"A=a\nCHROMEOS_RELEASE_BOARD=foo\nCHROMEOS_RELEASE_BUILDER_PATH=bar/baz\nB=b\n", "foo", "bar/baz"},
	} {
		info := Parse(tc.data)
		if info.Board != tc.board {
			t.Errorf("Parse(%q).Board = %q; want %q", tc.data, info.Board, tc.board)
		}
		if info.BuilderPath != tc.builderPath {
			t.Errorf("Parse(%q).BuilderPath = %q; want %q", tc.data, info.BuilderPath, tc.builderPath)
		}
	}
}
