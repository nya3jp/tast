// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcheck provides common functions to check test definitions.
package testcheck

import (
	"fmt"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/internal/testing"
)

// SetAllTestsforTest sets all tests to use in this package. This is mainly used in unittest for testing purpose.
func SetAllTestsforTest(tests []*testing.TestInstance) func() {
	allTests = func() []*testing.TestInstance {
		return tests
	}
	return func() {
		allTests = testing.GlobalRegistry().AllTests
	}
}

// TestFilter defines the condition whether or not the test should be checked.
type TestFilter func(t *testing.TestInstance) bool

var allTests func() []*testing.TestInstance = testing.GlobalRegistry().AllTests

func getTests(t *gotesting.T, f TestFilter) []*testing.TestInstance {
	var tests []*testing.TestInstance
	for _, tst := range allTests() {
		if f(tst) {
			tests = append(tests, tst)
		}
	}
	if len(tests) == 0 {
		t.Fatalf("No tests matched")
	}
	return tests
}

// Glob returns a TestFilter which returns true for a test if the test name
// matches with the given glob pattern.
func Glob(t *gotesting.T, glob string) TestFilter {
	re, err := testing.NewTestGlobRegexp(glob)
	if err != nil {
		t.Fatalf("Bad glob %q: %v", glob, err)
	}
	return func(t *testing.TestInstance) bool {
		return re.MatchString(t.Name)
	}
}

// Timeout checks that tests matched by f have timeout no less than minTimeout.
func Timeout(t *gotesting.T, f TestFilter, minTimeout time.Duration) {
	for _, tst := range getTests(t, f) {
		if tst.Timeout < minTimeout {
			t.Errorf("%s: timeout is too short (%v < %v)", tst.Name, tst.Timeout, minTimeout)
		}
	}
}

// Attr checks that tests matched by f declare requiredAttr as Attr.
// requiredAttr is a list of items which the test's Attr must have.
// Each item is one or '|'-connected multiple attr names, and Attr must contain at least one of them.
func Attr(t *gotesting.T, f TestFilter, requiredAttr []string) {
	for _, tst := range getTests(t, f) {
		attr := make(map[string]struct{})
		for _, at := range tst.Attr {
			attr[at] = struct{}{}
		}
	CheckLoop:
		for _, at := range requiredAttr {
			for _, item := range strings.Split(at, "|") {
				if _, ok := attr[item]; ok {
					continue CheckLoop
				}
			}
			t.Errorf("%s: missing attribute %q", tst.Name, at)
		}
	}
}

// IfAttr checks that tests matched by f declare requiredAttr as Attr if all Attr in criteriaAttr are present.
// criteriaAttr is a list of items to apply to test's Attr.
// requiredAttr is a list of items which the test's Attr must have if criteriaAttr are matched.
// Each item is one or '|'-connected multiple attr names, and Attr must contain at least one of them.
// Example, criteriaAttr=["A", "B|C"], requiredAttr=["D", "E|F"]
// Any tests with Attr A and either B or C should define Attr D and either E or F.
func IfAttr(t *gotesting.T, f TestFilter, criteriaAttr, requiredAttr []string) {
TestLoop:
	for _, tst := range getTests(t, f) {
		attr := make(map[string]struct{})
		for _, at := range tst.Attr {
			attr[at] = struct{}{}
		}
	CriteriaLoop:
		for _, at := range criteriaAttr {
			for _, item := range strings.Split(at, "|") {
				if _, ok := attr[item]; ok {
					continue CriteriaLoop
				}
			}
			// Any one of the criteria is not met.
			continue TestLoop
		}
	CheckLoop:
		for _, at := range requiredAttr {
			for _, item := range strings.Split(at, "|") {
				if _, ok := attr[item]; ok {
					continue CheckLoop
				}
			}
			t.Errorf("%s: missing attribute %q", tst.Name, at)
		}
	}
}

// SoftwareDeps checks that tests matched by f declare requiredDeps as software dependencies.
// requiredDeps is a list of items which the test's SoftwareDeps needs to
// satisfy. Each item is one or '|'-connected multiple software feature names,
// and SoftwareDeps must contain at least one of them.
func SoftwareDeps(t *gotesting.T, f TestFilter, requiredDeps []string) {
	for _, tst := range getTests(t, f) {
		deps := make(map[string]struct{})
		for _, d := range tst.SoftwareDeps {
			deps[d] = struct{}{}
		}
	CheckLoop:
		for _, d := range requiredDeps {
			for _, item := range strings.Split(d, "|") {
				if _, ok := deps[item]; ok {
					continue CheckLoop
				}
			}
			t.Errorf("%s: missing software dependency %q", tst.Name, d)
		}
	}
}

// EntityType represents a type of an entity, such as a test or a fixture.
type EntityType int

const (
	// Test represents that an entity is a test.
	Test EntityType = iota
	// Fixture represents that an entity is a fixture.
	Fixture
)

func (e EntityType) String() string {
	switch e {
	case Test:
		return "test"
	case Fixture:
		return "fixture"
	default:
		return fmt.Sprintf("unknown(%d)", e)
	}
}

// Entity represents a node in the dependency graph of tests and fixtures.
type Entity struct {
	Type   EntityType
	Name   string
	Parent string
	Labels map[string]struct{}
	Attrs  map[string]struct{}
}

// HasLabel returns whether the node has given label.
func (e *Entity) HasLabel(name string) bool {
	_, ok := e.Labels[name]
	return ok
}

// HasAttr returns whether the node has given Attr.
func (e *Entity) HasAttr(name string) bool {
	_, ok := e.Attrs[name]
	return ok
}

// Entities gives all dependency data of all tests.
func Entities() map[string]Entity {
	stringSet := func(list []string) map[string]struct{} {
		m := make(map[string]struct{})
		for _, v := range list {
			m[v] = struct{}{}
		}
		return m
	}
	result := make(map[string]Entity)
	for _, tst := range allTests() {
		result[tst.Name] = Entity{
			Type:   Test,
			Name:   tst.Name,
			Parent: tst.Fixture,
			Labels: stringSet(tst.Labels),
			Attrs:  stringSet(tst.Attr),
		}
	}
	for n, f := range testing.GlobalRegistry().AllFixtures() {
		result[n] = Entity{
			Type:   Fixture,
			Name:   n,
			Parent: f.Parent,
			Labels: stringSet(f.Labels),
			Attrs:  nil,
		}
	}
	return result
}
