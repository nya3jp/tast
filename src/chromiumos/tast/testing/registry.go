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
	allTests []*Test
	autoName bool // true if test names should be automatically derived from func names
}

type registryOption func(*Registry)

// NoAutoName can be passed to NewRegistry to configure the returned registry to skip automatically
// assigning names to tests and checking that each test's function name matches the name of the file
// that registered it. This is used by unit tests that want to add tests with test function names
// that don't match the test file's name (e.g. a file named "file_test.go" would typically be expected
// to register a test function with a name like "FileTest").
var NoAutoName = func(r *Registry) {
	r.autoName = false
}

// NewRegistry returns a new test registry.
func NewRegistry(opts ...registryOption) *Registry {
	r := &Registry{
		allTests: make([]*Test, 0),
		autoName: true,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// AddTest adds t to the registry. Missing fields are filled where possible.
func (r *Registry) AddTest(t *Test) error {
	if err := t.finalize(r.autoName); err != nil {
		return err
	}
	r.allTests = append(r.allTests, t)
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

// testsForPattern returns registered tests with names matched by p,
// a pattern that may contain '*' wildcards.
func (r *Registry) testsForPattern(p string) ([]*Test, error) {
	if err := validateTestPattern(p); err != nil {
		return nil, fmt.Errorf("bad pattern %q: %v", p, err)
	}
	p = strings.Replace(p, ".", "\\.", -1)
	p = strings.Replace(p, "*", ".*", -1)
	p = "^" + p + "$"
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, fmt.Errorf("failed to compile %q: %v", p, err)
	}

	tests := make([]*Test, 0)
	for _, t := range r.allTests {
		if re.MatchString(t.Name) {
			tests = append(tests, t)
		}
	}
	return tests, nil
}

// validateTestPattern returns an error if n contains one or more characters
// disallowed in test wildcard patterns.
func validateTestPattern(p string) error {
	for _, ch := range p {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '.' && ch != '*' {
			return fmt.Errorf("invalid character %v", ch)
		}
	}
	return nil
}

// TestsForPatterns de-duplicates and returns copies of registered tests with names matched by
// any pattern in ps.
func (r *Registry) TestsForPatterns(ps []string) ([]*Test, error) {
	tests := make([]*Test, 0)
	seen := make(map[*Test]struct{})
	for _, p := range ps {
		ts, err := r.testsForPattern(p)
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
