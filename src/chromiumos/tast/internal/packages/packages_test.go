// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package packages_test

import (
	"path/filepath"
	"testing"

	"chromiumos/tast/internal/packages"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{{
		filepath.Join(packages.OldFrameworkPrefix, "foo"),
		filepath.Join(packages.FrameworkPrefix, "foo"),
	}, {
		filepath.Join(packages.FrameworkPrefix, "foo"),
		filepath.Join(packages.FrameworkPrefix, "foo"),
	}, {
		"foo",
		"foo",
	}} {
		if got := packages.Normalize(tc.input); got != tc.want {
			t.Errorf("TrimCommonPrefix(%q) = %q want %q", tc.input, got, tc.want)
		}
	}
}

func TestSplitFuncName(t *testing.T) {
	for _, pkg := range []string{
		filepath.Join(packages.FrameworkPrefix, "foo"),
		filepath.Join(packages.OldFrameworkPrefix, "foo"),
	} {
		fn := pkg + ".Bar"
		gotPkg, gotName := packages.SplitFuncName(fn)
		if gotPkg != pkg {
			t.Errorf("SplitFuncName(%q).0 = %q want %q", fn, gotPkg, pkg)
		}
		if gotName != "Bar" {
			t.Errorf("SplitFuncName(%q).1 = %q want %q", fn, gotName, "Bar")
		}
	}
}
