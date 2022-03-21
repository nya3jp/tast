// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides public API for tests.
package testing

import (
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// Test describes a registration of one or more test instances.
//
// Test can be passed to testing.AddTest to actually register test instances
// to the framework.
//
// In the most basic form where Params field is empty, Test describes exactly
// one test instance. If Params is not empty, multiple test instances are
// generated on registration by merging each testing.Param to the base Test.
type Test = testing.Test

// Param defines parameters for a parameterized test case.
// See also https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Parameterized-tests
type Param = testing.Param

// TestInstance represents a test instance registered to the framework.
//
// A test instance is the unit of "tests" exposed to outside of the framework.
// For example, in the command line of the "tast" command, users specify
// which tests to run by names of test instances. Single testing.AddTest call
// may register multiple test instances at once if testing.Test passed to the
// function has non-empty Params field.
type TestInstance = testing.TestInstance

const (
	// LacrosVariantUnknown indicates that this test has not yet been checked as to whether it requires a lacros variant.
	// New tests should not use this value, i.e. new tests should always consider lacros.
	LacrosVariantUnknown = testing.LacrosVariantUnknown
	// LacrosVariantNeeded indicates that a lacros variant for this is needed but hasn't been created yet.
	LacrosVariantNeeded = testing.LacrosVariantNeeded
	// LacrosVariantExists indicates that all required lacros variants for this test have been created.
	LacrosVariantExists = testing.LacrosVariantExists
	// LacrosVariantUnneeded indicates that lacros variants for this test are not needed.
	LacrosVariantUnneeded = testing.LacrosVariantUnneeded
)

// StringPair represents a string key-value pair. Typically used for SearchFlags.
type StringPair = protocol.StringPair
