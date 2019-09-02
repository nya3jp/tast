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
	allTests  []*TestCase
	testNames map[string]struct{} // names of registered tests
}

// NewRegistry returns a new test registry.
func NewRegistry() *Registry {
	return &Registry{
		allTests:  make([]*TestCase, 0),
		testNames: make(map[string]struct{}),
	}
}

// AddTest adds t to the registry.
func (r *Registry) AddTest(t *Test) error {
	if err := validateTest(t); err != nil {
		return err
	}
	if len(t.Params) == 0 {
		tc, err := newTestCase(t, nil)
		if err != nil {
			return err
		}
		return r.AddTestCase(tc)
	}

	for _, p := range t.Params {
		tc, err := newTestCase(t, &p)
		if err != nil {
			return err
		}
		if err := r.AddTestCase(tc); err != nil {
			return err
		}
	}
	return nil
}

// AddTestCase adds t to the registry.
// TODO(crbug.com/985381): Consider to hide the method for better encapsulation.
func (r *Registry) AddTestCase(t *TestCase) error {
	t = t.clone()
	if _, ok := r.testNames[t.Name]; ok {
		return fmt.Errorf("test %q already registered", t.Name)
	}
	r.allTests = append(r.allTests, t)
	r.testNames[t.Name] = struct{}{}
	return nil
}

// AllTests returns copies of all registered tests.
func (r *Registry) AllTests() []*TestCase {
	ts := make([]*TestCase, len(r.allTests))
	for i, t := range r.allTests {
		ts[i] = t.clone()
	}
	return ts
}

// testsForGlob returns registered tests with names matched by w,
// which may contain '*' to match zero or more arbitrary characters.
func (r *Registry) testsForGlob(g string) ([]*TestCase, error) {
	re, err := NewTestGlobRegexp(g)
	if err != nil {
		return nil, fmt.Errorf("bad glob %q: %v", g, err)
	}

	tests := make([]*TestCase, 0)
	for _, t := range r.allTests {
		if re.MatchString(t.Name) {
			tests = append(tests, t)
		}
	}
	return tests, nil
}

// TestsForGlobs de-duplicates and returns copies of registered tests with names matched by
// any glob in gs. See NewTestGlobRegexp for details about the glob format.
func (r *Registry) TestsForGlobs(gs []string) ([]*TestCase, error) {
	tests := make([]*TestCase, 0)
	seen := make(map[*TestCase]struct{})
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
func (r *Registry) TestsForAttrExpr(s string) ([]*TestCase, error) {
	expr, err := expr.New(s)
	if err != nil {
		return nil, fmt.Errorf("bad expr: %v", err)
	}

	tests := make([]*TestCase, 0)
	for _, t := range r.allTests {
		if expr.Matches(t.Attr) {
			tests = append(tests, t.clone())
		}
	}
	return tests, nil
}

// NewTestGlobRegexp returns a compiled regular expression corresponding to g,
// a glob for matching test names. g may consist of letters, digits, periods,
// underscores and '*' to match zero or more arbitrary characters.
//
// This matches the logic used by TestsForGlobs and is exported to make it possible
// for code outside this package to verify that a user-supplied glob matched at least one test.
func NewTestGlobRegexp(g string) (*regexp.Regexp, error) {
	for _, ch := range g {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '.' && ch != '_' && ch != '*' {
			return nil, fmt.Errorf("invalid character %v", ch)
		}
	}
	g = strings.Replace(g, ".", "\\.", -1)
	g = strings.Replace(g, "*", ".*", -1)
	g = "^" + g + "$"
	return regexp.Compile(g)
}
