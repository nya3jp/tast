// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcheck provides common functions to check test definitions.
package testcheck

import (
	gotesting "testing"
	"time"

	"go.chromium.org/tast/core/tastuseonly/testing"
	"go.chromium.org/tast/core/testing/testcheck"
)

// SetAllTestsforTest sets all tests to use in this package. This is mainly used in unittest for testing purpose.
func SetAllTestsforTest(tests []*testing.TestInstance) func() {
	return testcheck.SetAllTestsforTest(tests)
}

// TestFilter defines the condition whether or not the test should be checked.
type TestFilter = testcheck.TestFilter

// Glob returns a TestFilter which returns true for a test if the test name
// matches with the given glob pattern.
func Glob(t *gotesting.T, glob string) TestFilter {
	return testcheck.Glob(t, glob)
}

// Timeout checks that tests matched by f have timeout no less than minTimeout.
func Timeout(t *gotesting.T, f TestFilter, minTimeout time.Duration) {
	testcheck.Timeout(t, f, minTimeout)
}

// Attr checks that tests matched by f declare requiredAttr as Attr.
// requiredAttr is a list of items which the test's Attr must have.
// Each item is one or '|'-connected multiple attr names, and Attr must contain at least one of them.
func Attr(t *gotesting.T, f TestFilter, requiredAttr []string) {
	testcheck.Attr(t, f, requiredAttr)
}

// IfAttr checks that tests matched by f declare requiredAttr as Attr if all Attr in criteriaAttr are present.
// criteriaAttr is a list of items to apply to test's Attr.
// requiredAttr is a list of items which the test's Attr must have if criteriaAttr are matched.
// Each item is one or '|'-connected multiple attr names, and Attr must contain at least one of them.
// Example, criteriaAttr=["A", "B|C"], requiredAttr=["D", "E|F"]
// Any tests with Attr A and either B or C should define Attr D and either E or F.
func IfAttr(t *gotesting.T, f TestFilter, criteriaAttr, requiredAttr []string) {
	testcheck.IfAttr(t, f, criteriaAttr, requiredAttr)
}

// SoftwareDeps checks that tests matched by f declare requiredDeps as software dependencies.
// requiredDeps is a list of items which the test's SoftwareDeps needs to
// satisfy. Each item is one or '|'-connected multiple software feature names,
// and SoftwareDeps must contain at least one of them.
// TODO: b/225978622 -- support multi-dut in test check.
func SoftwareDeps(t *gotesting.T, f TestFilter, requiredDeps []string) {
	testcheck.SoftwareDeps(t, f, requiredDeps)
}

// EntityType represents a type of an entity, such as a test or a fixture.
type EntityType = testcheck.EntityType

const (
	// Test represents that an entity is a test.
	Test EntityType = testcheck.Test
	// Fixture represents that an entity is a fixture.
	Fixture = testcheck.Fixture
)

// Entity represents a node in the dependency graph of tests and fixtures.
type Entity = testcheck.Entity

// Entities gives all dependency data of all tests.
func Entities() map[string]Entity {
	return testcheck.Entities()
}
