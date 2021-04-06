// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

var globalRegistry *Registry // singleton, initialized on first use

// GlobalRegistry returns a global registry containing tests
// registered by calls to AddTest.
func GlobalRegistry() *Registry {
	if globalRegistry == nil {
		globalRegistry = NewRegistry()
	}
	return globalRegistry
}

// AddTest adds test t to the global registry.
func AddTest(t *Test) {
	GlobalRegistry().AddTest(t)
}

// AddTestInstance adds test case t to the global registry. This is only for
// testing purpose.
func AddTestInstance(t *TestInstance) {
	GlobalRegistry().AddTestInstance(t)
}

// AddService adds service s to the global registry.
func AddService(s *Service) {
	GlobalRegistry().AddService(s)
}

// AddFixture adds fixture f to the global registry.
func AddFixture(f *Fixture) {
	GlobalRegistry().AddFixture(f)
}

// SetGlobalRegistryForTesting temporarily sets reg as the global registry and clears registration errors.
// The caller must call the returned function later to restore the original registry and errors.
// This is intended to be used by unit tests that need to register tests in the global registry but don't
// want to affect subsequent unit tests.
func SetGlobalRegistryForTesting(reg *Registry) (restore func()) {
	origReg := globalRegistry
	globalRegistry = reg
	return func() {
		globalRegistry = origReg
	}
}
