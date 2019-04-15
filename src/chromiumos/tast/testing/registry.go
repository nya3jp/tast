// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"chromiumos/tast/expr"
)

// Registry holds tests.
type Registry struct {
	allTests  []*Test
	testNames map[string]struct{} // names of registered tests
	finalize  bool                // call Test.finalize to validate and automatically set fields
	autoName  bool                // automatically derive test names from func names
}

type registryOption func(*Registry)

// NoAutoName can be passed to NewRegistry to configure the returned registry to skip automatically
// assigning names to tests and checking that each test's function name matches the name of the file
// that registered it. This is used by unit tests that want to add tests with test function names
// that don't match the test file's name (e.g. a file named "file_test.go" would typically be expected
// to register a test function with a name like "FileTest").
var NoAutoName = func(r *Registry) { r.autoName = false }

// NoFinalize can be passed to NewRegistry to configure the returned registry to not perform any
// validation or automatic field-filling when tests are added via Addtest. This option implies NoAutoName.
// This should only be used when importing serialized tests that will not actually be run later.
var NoFinalize = func(r *Registry) { r.finalize = false }

// NewRegistry returns a new test registry.
func NewRegistry(opts ...registryOption) *Registry {
	r := &Registry{
		allTests:  make([]*Test, 0),
		testNames: make(map[string]struct{}),
		finalize:  true,
		autoName:  true,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// AddTest adds t to the registry. Missing fields are filled where possible.
func (r *Registry) AddTest(t *Test) error {
	// Copy the test to ensure that later changes made by the caller don't affect us.
	t = t.clone()
	if r.finalize {
		if err := t.finalize(r.autoName); err != nil {
			return err
		}
	}
	if _, ok := r.testNames[t.Name]; ok {
		return fmt.Errorf("test %q already registered", t.Name)
	}
	r.allTests = append(r.allTests, t)
	r.testNames[t.Name] = struct{}{}
	return nil
}

// AllTests returns copies of all registered tests.
func (r *Registry) AllTests() []*Test {
	ts := make([]*Test, len(r.allTests))
	for i, t := range r.allTests {
		ts[i] = t.clone()
	}
	return ts
}

// testsForGlob returns registered tests with names matched by w,
// which may contain '*' to match zero or more arbitrary characters.
func (r *Registry) testsForGlob(g string) ([]*Test, error) {
	if err := validateTestGlob(g); err != nil {
		return nil, fmt.Errorf("bad glob %q: %v", g, err)
	}
	g = strings.Replace(g, ".", "\\.", -1)
	g = strings.Replace(g, "*", ".*", -1)
	g = "^" + g + "$"
	re, err := regexp.Compile(g)
	if err != nil {
		return nil, fmt.Errorf("failed to compile %q: %v", g, err)
	}

	tests := make([]*Test, 0)
	for _, t := range r.allTests {
		if re.MatchString(t.Name) {
			tests = append(tests, t)
		}
	}
	return tests, nil
}

// validateTestGlob returns an error if g contains one or more characters
// disallowed in test glob patterns.
func validateTestGlob(g string) error {
	for _, ch := range g {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '.' && ch != '*' {
			return fmt.Errorf("invalid character %v", ch)
		}
	}
	return nil
}

// TestsForGlobs de-duplicates and returns copies of registered tests with names matched by
// any glob in gs.
func (r *Registry) TestsForGlobs(gs []string) ([]*Test, error) {
	tests := make([]*Test, 0)
	seen := make(map[*Test]struct{})
	for _, g := range gs {
		ts, err := r.testsForGlob(g)
		if err != nil {
			return nil, err
		}

		// De-dupe results while preserving order.
		for _, t := range ts {
			if _, ok := seen[t]; ok {
				continue
			}
			tests = append(tests, t.clone())
			seen[t] = struct{}{}
		}
	}
	return tests, nil
}

// TestsForAttrExpr returns copies of registered tests with attributes matched by s,
// a boolean expression of attributes, e.g. "(attr1 && !attr2) || attr3".
// See chromiumos/tast/expr for details about expression syntax.
func (r *Registry) TestsForAttrExpr(s string) ([]*Test, error) {
	expr, err := expr.New(s)
	if err != nil {
		return nil, fmt.Errorf("bad expr: %v", err)
	}

	tests := make([]*Test, 0)
	for _, t := range r.allTests {
		if expr.Matches(t.Attr) {
			tests = append(tests, t.clone())
		}
	}
	return tests, nil
}
