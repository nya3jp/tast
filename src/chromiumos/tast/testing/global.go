// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"regexp"
	"runtime"

	"chromiumos/tast/internal/testing"
)

// verifier is a global singleton to check if AddTest() is used as designed.
var verifier = newCallerVerifier(
	regexp.MustCompile(`^chromiumos/tast/(local|remote)/bundles/[^/]+/[^/]+\.init.\d+$`))

// RegistrationErrors returns errors generated by calls to AddTest.
func RegistrationErrors() []error {
	return testing.RegistrationErrors()
}

// AddTest adds test t to the global registry.
// This should be called only once in a test main file's init(),
// and it should be the top level statement of the init()'s body.
// The argument of AddTest() in the case should be a pointer to a
// composite literal of testing.Test.
func AddTest(t *Test) {
	pc, _, _, _ := runtime.Caller(1)
	verifier.verifyAndRegister(pc)
	testing.AddTest(t)
}

// AddService adds service s to the global registry.
// This should be called only once in a service main file's init().
func AddService(s *Service) {
	testing.AddService(s)
}
