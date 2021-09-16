// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"regexp"
	gotesting "testing"

	"chromiumos/tast/internal/caller"
)

func TestFuncNameVerify(t *gotesting.T) {
	v := newCallerVerifier(regexp.MustCompile("^go.chromium.org/tast/testing.TestFuncNameVerify$"))
	if err := v.verifyAndRegister(caller.Func(1)); err != nil {
		t.Error("Unexpected verification failure: ", err)
	}

	v = newCallerVerifier(regexp.MustCompile("^go.chromium.org/tast/testing.NoMatchFuncName$"))
	if err := v.verifyAndRegister(caller.Func(1)); err == nil {
		t.Error("Unexpected verification pass for unmatched function name")
	}
}

func TestRegisterTwiceVerify(t *gotesting.T) {
	v := newCallerVerifier(regexp.MustCompile(".*"))
	if err := v.verifyAndRegister(caller.Func(1)); err != nil {
		t.Fatal("Unexpected verification failure: ", err)
	}
	if err := v.verifyAndRegister(caller.Func(1)); err == nil {
		t.Fatal("Unexpected verification pass for two times registration")
	}
}
