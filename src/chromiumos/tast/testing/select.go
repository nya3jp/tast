// Copyright 2019 The Chromium OS Authors. All rights reserved.
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

// TestPatternType describes the manner in which test patterns will be interpreted.
type TestPatternType int

const (
	// TestPatternGlobs means the patterns will be interpreted as one or more globs (possibly literal test names).
	TestPatternGlobs TestPatternType = iota
	// TestPatternAttrExpr means the patterns will be interpreted as a boolean expression referring to test attributes.
	TestPatternAttrExpr
)

// GetTestPatternType returns the manner in which test patterns pats will be interpreted.
// This is exported so it can be used by the tast command.
func GetTestPatternType(pats []string) TestPatternType {
	switch {
	case len(pats) == 1 && strings.HasPrefix(pats[0], "(") && strings.HasSuffix(pats[0], ")"):
		return TestPatternAttrExpr
	default:
		return TestPatternGlobs
	}
}

// SelectTestsByArgs returns a subset of tests filtered by arguments given to
// runners or bundles.
//
// If no args are supplied, all tests are returned.
//
// If a single arg is supplied and it is surrounded by parentheses,
// it is treated as a boolean expression specifying test attributes.
//
// Otherwise, arg(s) are interpreted as globs matching test names.
func SelectTestsByArgs(tests []*TestCase, args []string) ([]*TestCase, error) {
	switch GetTestPatternType(args) {
	case TestPatternGlobs:
		if len(args) == 0 {
			return tests, nil
		}
		// Print a helpful error message if it looks like the user wanted an attribute expression.
		if len(args) == 1 && (strings.Contains(args[0], "&&") || strings.Contains(args[0], "||")) {
			return nil, fmt.Errorf("attr expr %q must be within parentheses", args[0])
		}
		return SelectTestsByGlobs(tests, args)
	case TestPatternAttrExpr:
		return SelectTestsByAttrExpr(tests, args[0][1:len(args[0])-1])
	}
	return nil, fmt.Errorf("invalid test pattern(s) %v", args)
}

// selectTestsByGlob returns a subset of tests with names matched by w,
// which may contain '*' to match zero or more arbitrary characters.
func selectTestsByGlob(tests []*TestCase, g string) ([]*TestCase, error) {
	re, err := NewTestGlobRegexp(g)
	if err != nil {
		return nil, fmt.Errorf("bad glob %q: %v", g, err)
	}

	var filtered []*TestCase
	for _, t := range tests {
		if re.MatchString(t.Name) {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// SelectTestsByGlobs de-duplicates and returns a subset of tests with names matched by
// any glob in gs. See NewTestGlobRegexp for details about the glob format.
func SelectTestsByGlobs(tests []*TestCase, gs []string) ([]*TestCase, error) {
	var filtered []*TestCase
	seen := make(map[*TestCase]struct{})
	for _, g := range gs {
		ts, err := selectTestsByGlob(tests, g)
		if err != nil {
			return nil, err
		}

		// De-dupe results while preserving order.
		for _, t := range ts {
			if _, ok := seen[t]; ok {
				continue
			}
			filtered = append(filtered, t.clone())
			seen[t] = struct{}{}
		}
	}
	return filtered, nil
}

// SelectTestsByAttrExpr returns a subset of tests with attributes matched by s,
// a boolean expression of attributes, e.g. "(attr1 && !attr2) || attr3".
// See chromiumos/tast/expr for details about expression syntax.
func SelectTestsByAttrExpr(tests []*TestCase, s string) ([]*TestCase, error) {
	expr, err := expr.New(s)
	if err != nil {
		return nil, fmt.Errorf("bad expr: %v", err)
	}

	var filtered []*TestCase
	for _, t := range tests {
		if expr.Matches(t.Attr) {
			filtered = append(filtered, t.clone())
		}
	}
	return filtered, nil
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
