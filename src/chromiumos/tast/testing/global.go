// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"runtime"
)

var globalRegistry *Registry   // singleton, initialized on first use
var registrationErrors []error // singleton for errors encountered in AddTest calls

// verifier is a global singleton to check if AddTest() is used as designed.
var verifier = newCallerVerifier(
	regexp.MustCompile(`^chromiumos/tast/(local|remote)/bundles/[^/]+/[^/]+\.init.\d+$`))

// GlobalRegistry returns a global registry containing tests
// registered by calls to AddTest.
func GlobalRegistry() *Registry {
	if globalRegistry == nil {
		globalRegistry = NewRegistry()
	}
	return globalRegistry
}

// RegistrationErrors returns errors generated by calls to AddTest.
func RegistrationErrors() []error {
	return registrationErrors
}

// AddTest adds test t to the global registry.
// This should be called only once in a test main file's init(),
// and it should be the top level statement of the init()'s body.
// The argument of AddTest() in the case should be a pointer to a
// composite literal of testing.Test.
func AddTest(t *Test) {
	pc, file, line, _ := runtime.Caller(1)
	if err := addTestInternal(t, pc); err != nil {
		registrationErrors = append(registrationErrors, fmt.Errorf("%s:%d: %v", file, line, err))
	}
}

func addTestInternal(t *Test, pc uintptr) error {
	if verifier != nil {
		if err := verifier.verifyAndRegister(pc); err != nil {
			return err
		}
	}
	if err := GlobalRegistry().AddTest(t); err != nil {
		return err
	}
	return nil
}

// AddTestCase adds test case t to the global registry. This is only for
// testing purpose.
func AddTestCase(t *TestCase) {
	if err := GlobalRegistry().AddTestCase(t); err != nil {
		_, file, line, _ := runtime.Caller(1)
		registrationErrors = append(registrationErrors, fmt.Errorf("%s:%d: %v", file, line, err))
	}
}

// SetGlobalRegistryForTesting temporarily sets reg as the global registry and clears registration errors.
// The caller must call the returned function later to restore the original registry and errors.
// This is intended to be used by unit tests that need to register tests in the global registry but don't
// want to affect subsequent unit tests.
func SetGlobalRegistryForTesting(reg *Registry) (restore func()) {
	origReg := globalRegistry
	origErrs := registrationErrors
	origVerifier := verifier

	globalRegistry = reg
	registrationErrors = nil
	verifier = nil

	return func() {
		verifier = origVerifier
		registrationErrors = origErrs
		globalRegistry = origReg
	}
}
