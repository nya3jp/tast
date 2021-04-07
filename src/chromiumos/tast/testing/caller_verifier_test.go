// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"regexp"
	"runtime"
	gotesting "testing"
)

func TestFuncNameVerify(t *gotesting.T) {
	v := newCallerVerifier(regexp.MustCompile("^chromiumos/tast/testing.TestFuncNameVerify$"))
	pc, _, _, _ := runtime.Caller(0)
	if err := v.verifyAndRegister(pc); err != nil {
		t.Error("Unexpected verification failure: ", err)
	}

	v = newCallerVerifier(regexp.MustCompile("^chromiumos/tast/testing.NoMatchFuncName$"))
	if err := v.verifyAndRegister(pc); err == nil {
		t.Error("Unexpected verification pass for unmatched function name")
	}
}

func TestRegisterTwiceVerify(t *gotesting.T) {
	v := newCallerVerifier(regexp.MustCompile(".*"))
	pc, _, _, _ := runtime.Caller(0)
	if err := v.verifyAndRegister(pc); err != nil {
		t.Fatal("Unexpected verification failure: ", err)
	}
	if err := v.verifyAndRegister(pc); err == nil {
		t.Fatal("Unexpected verification pass for two times registration")
	}
}

func TestPackageFromPC(t *gotesting.T) {
	pc, _, _, _ := runtime.Caller(0)
	if got, want := packageForPC(pc), "chromiumos/tast/testing"; got != want {
		t.Errorf("packageFromPC(%#v) = %q, want %q", pc, got, want)
	}
}
