// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package packages_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"go.chromium.org/tast/core/internal/packages"
)

func callerFuncName(t *testing.T) string {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return runtime.FuncForPC(pc).Name()
}

type someStruct struct{}

func (*someStruct) method(t *testing.T) string {
	return callerFuncName(t)
}

func TestSplitFuncName(t *testing.T) {
	fn1 := callerFuncName(t)
	fn2 := (&someStruct{}).method(t)
	fn3 := func() string {
		return callerFuncName(t)
	}()

	for _, tc := range []struct {
		fn                string
		wantNormalizedPkg string
		wantName          string
	}{
		{"go.chromium.org/tast/foo.Bar", "go.chromium.org/tast/foo", "Bar"},
		{fn1, "go.chromium.org/tast/core/internal/packages_test", "TestSplitFuncName"},
		{fn2, "go.chromium.org/tast/core/internal/packages_test", "(*someStruct).method"},
		{fn3, "go.chromium.org/tast/core/internal/packages_test", "TestSplitFuncName.func1"},
	} {
		t.Run(tc.fn, func(t *testing.T) {
			gotPkg, gotName := packages.SplitFuncName(tc.fn)
			if got := packages.Normalize(gotPkg); got != tc.wantNormalizedPkg {
				t.Errorf("Got normalized package %q want %q", got, tc.wantNormalizedPkg)
			}
			if gotName != tc.wantName {
				t.Errorf("Got name %q want %q", gotName, tc.wantName)
			}
		})
	}
}

func TestSame(t *testing.T) {
	var (
		foo      = filepath.Join(packages.FrameworkPrefix, "foo")
		otherFoo = "foo"
		bar      = filepath.Join(packages.FrameworkPrefix, "bar")
	)

	for _, tc := range []struct {
		x    string
		y    string
		want bool
	}{
		{foo, foo, true},
		{foo, otherFoo, false},
		{foo, bar, false},
	} {
		if got := packages.Same(tc.x, tc.y); got != tc.want {
			t.Errorf("Same(%q, %q) = %v want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestSrcPathInTastRepo(t *testing.T) {
	for _, tc := range []struct {
		fn       string
		wantPath string
	}{
		{"abcde/platform/tast/src/go.chromium.org/tast/foo.Bar",
			"src/go.chromium.org/tast/foo.Bar"},
		{"abcee/platform/tast-tests/src/go.chromium.org/tast-tests",
			"src/go.chromium.org/tast-tests"},
		{"abcee/platform/tast-tests-private/src/go.chromium.org/tast-tests-private",
			"src/go.chromium.org/tast-tests-private"},
		{"abcee/partner-intel-private/src/go.chromium.org/partner-intel-private",
			"src/go.chromium.org/partner-intel-private"},
		{"abcee/abcd",
			"abcee/abcd"},
	} {
		t.Run(tc.fn, func(t *testing.T) {
			gotPath := packages.SrcPathInTastRepo(tc.fn)
			if gotPath != tc.wantPath {
				t.Errorf("Got name %q want %q", gotPath, tc.wantPath)
			}
		})
	}
}
