// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"chromiumos/tast/internal/expr"
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

// validatePattern check if test pattern has wildcard and it will return error if given pattern is not a valild test pattern.
func validatePattern(pat string) (hasWildcard bool, err error) {
	for _, ch := range pat {
		switch {
		case ch == '*':
			hasWildcard = true
		case unicode.IsLetter(ch), unicode.IsDigit(ch), ch == '.', ch == '_':
			continue
		default:
			return hasWildcard, fmt.Errorf("invalid character %v", ch)
		}
	}
	return hasWildcard, nil
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
func SelectTestsByArgs(tests []*TestInstance, args []string) ([]*TestInstance, error) {
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
func selectTestsByGlob(tests map[string]*TestInstance, g string) ([]*TestInstance, error) {
	re, err := NewTestGlobRegexp(g)
	if err != nil {
		return nil, fmt.Errorf("bad glob %q: %v", g, err)
	}

	var filtered []*TestInstance
	for _, t := range tests {
		if re.MatchString(t.Name) {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// SelectTestsByGlobs de-duplicates and returns a subset of tests with names matched by
// any glob in gs. See NewTestGlobRegexp for details about the glob format.
func SelectTestsByGlobs(tests []*TestInstance, gs []string) ([]*TestInstance, error) {
	var filtered []*TestInstance
	unmatched := make(map[string](*TestInstance))
	for _, t := range tests {
		unmatched[t.Name] = t
	}
	for _, g := range gs {
		hasWildcard, err := validatePattern(g)
		if err != nil {
			return nil, err
		}
		if !hasWildcard {
			if t, ok := unmatched[g]; ok {
				filtered = append(filtered, t.clone())
				delete(unmatched, g)
			}
			continue
		}
		ts, err := selectTestsByGlob(unmatched, g)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			filtered = append(filtered, t.clone())
			delete(unmatched, t.Name)
		}
	}
	return filtered, nil
}

// SelectTestsByAttrExpr returns a subset of tests with attributes matched by s,
// a boolean expression of attributes, e.g. "(attr1 && !attr2) || attr3".
// See chromiumos/tast/internal/expr for details about expression syntax.
func SelectTestsByAttrExpr(tests []*TestInstance, s string) ([]*TestInstance, error) {
	expr, err := expr.New(s)
	if err != nil {
		return nil, fmt.Errorf("bad expr: %v", err)
	}

	var filtered []*TestInstance
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
	if _, err := validatePattern(g); err != nil {
		return nil, err
	}
	g = strings.Replace(g, ".", "\\.", -1)
	g = strings.Replace(g, "*", ".*", -1)
	g = "^" + g + "$"
	return regexp.Compile(g)
}
