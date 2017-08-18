// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"chromiumos/tast/common/testing/attr"
)

// Registry holds tests.
type Registry struct {
	allTests []*Test
}

// NewRegistry returns a new test registry.
func NewRegistry() *Registry {
	return &Registry{
		allTests: make([]*Test, 0),
	}
}

// AddTest adds t to the registry. Missing fields are filled where possible.
func (r *Registry) AddTest(t *Test) error {
	if err := t.populateNameAndPkg(); err != nil {
		return err
	}
	if err := validateTestName(t.Name); err != nil {
		return fmt.Errorf("invalid test name %q: %v", t.Name, err)
	}
	r.allTests = append(r.allTests, t)
	return nil
}

// AllTests returns all registered tests.
func (r *Registry) AllTests() []*Test {
	return append([]*Test{}, r.allTests...)
}

// testsForPattern returns registered tests with names matched by p,
// a pattern that may contain '*' wildcards.
func (r *Registry) testsForPattern(p string) ([]*Test, error) {
	if err := validateTestName(strings.Replace(p, "*", "", -1)); err != nil {
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

// TestsForPatterns de-duplicates and returns registered tests with names matched by
// any pattern in ps. An error is returned if any pattern matches zero tests.
func (r *Registry) TestsForPatterns(ps []string) ([]*Test, error) {
	tests := make([]*Test, 0)
	seen := make(map[*Test]struct{})
	for _, p := range ps {
		ts, err := r.testsForPattern(p)
		if err != nil {
			return nil, err
		}
		if len(ts) == 0 {
			return nil, fmt.Errorf("pattern %q didn't match any tests", p)
		}

		// De-dupe results while preserving order.
		for _, t := range ts {
			if _, ok := seen[t]; ok {
				continue
			}
			tests = append(tests, t)
			seen[t] = struct{}{}
		}
	}
	return tests, nil
}

// TestsForAttrExpr returns registered tests with attributes matched by s,
// a boolean expression of attributes, e.g. "(attr1 && !attr2) || attr3".
func (r *Registry) TestsForAttrExpr(s string) ([]*Test, error) {
	expr, err := attr.NewExpr(s)
	if err != nil {
		return nil, fmt.Errorf("bad expr: %v", err)
	}

	tests := make([]*Test, 0)
	for _, t := range r.allTests {
		if expr.Matches(t.Attr) {
			tests = append(tests, t)
		}
	}
	if len(tests) == 0 {
		return nil, fmt.Errorf("expr %q didn't match any tests", s)
	}
	return tests, nil
}

// validateTestName returns an error if n contains one or more characters disallowed in test names.
func validateTestName(n string) error {
	for _, ch := range n {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) &&
			ch != '-' && ch != '_' && ch != '.' {
			return fmt.Errorf("invalid character %v", ch)
		}
	}
	return nil
}
