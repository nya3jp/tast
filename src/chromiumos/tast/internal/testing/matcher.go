// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"chromiumos/tast/internal/expr"
)

// Matcher holds compiled patterns to match tests.
type Matcher struct {
	names map[string]struct{}
	globs map[string]*regexp.Regexp
	exprs []*expr.Expr
}

// NewMatcher creates a new Matcher from patterns.
func NewMatcher(pats []string) (*Matcher, error) {
	if len(pats) == 1 && strings.HasPrefix(pats[0], "(") && strings.HasSuffix(pats[0], ")") {
		return compileExpr(pats[0][1 : len(pats[0])-1])
	}
	return compileGlobs(pats)
}

// Match matches a test.
func (m *Matcher) Match(name string, attrs []string) bool {
	if _, ok := m.names[name]; ok {
		return true
	}
	for _, g := range m.globs {
		if g.MatchString(name) {
			return true
		}
	}
	for _, e := range m.exprs {
		if e.Matches(attrs) {
			return true
		}
	}
	return false
}

func compileExpr(s string) (*Matcher, error) {
	e, err := expr.New(s)
	if err != nil {
		return nil, fmt.Errorf("bad expr: %v", err)
	}
	return &Matcher{exprs: []*expr.Expr{e}}, nil
}

func compileGlobs(pats []string) (*Matcher, error) {
	// If the pattern is empty, return a matcher that matches anything.
	if len(pats) == 0 {
		pats = []string{"*"}
	}
	// Print a helpful error message if it looks like the user wanted an attribute expression.
	if len(pats) == 1 && (strings.Contains(pats[0], "&&") || strings.Contains(pats[0], "||")) {
		return nil, fmt.Errorf("attr expr %q must be within parentheses", pats[0])
	}

	names := make(map[string]struct{})
	globs := make(map[string]*regexp.Regexp)
	for _, pat := range pats {
		hasWildcard, err := validateGlob(pat)
		if err != nil {
			return nil, err
		}
		if hasWildcard {
			glob, err := compileGlob(pat)
			if err != nil {
				return nil, err
			}
			globs[pat] = glob
		} else {
			names[pat] = struct{}{}
		}
	}
	return &Matcher{names: names, globs: globs}, nil
}

// validateGlob checks if glob is a valid glob. It also returns if pat contains
// wildcards.
func validateGlob(glob string) (hasWildcard bool, err error) {
	for _, ch := range glob {
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

// compileGlob returns a compiled regular expression corresponding to glob.
// glob must be verified in advance with validateGlob.
func compileGlob(glob string) (*regexp.Regexp, error) {
	glob = strings.Replace(glob, ".", "\\.", -1)
	glob = strings.Replace(glob, "*", ".*", -1)
	glob = "^" + glob + "$"
	return regexp.Compile(glob)
}

// NewTestGlobRegexp returns a compiled regular expression corresponding to
// glob.
//
// DEPRECATED: Use Matcher instead.
func NewTestGlobRegexp(glob string) (*regexp.Regexp, error) {
	if _, err := validateGlob(glob); err != nil {
		return nil, err
	}
	return compileGlob(glob)
}

// UnmatchedPatterns returns a list of test name patterns (exact or wildcards) in the matcher that do not match any of supplied test names.
// This method always returns nil if the pattern in the matcher is an attribute expression.
func (m *Matcher) UnmatchedPatterns(tests []string) []string {
	if len(m.exprs) > 0 {
		return nil
	}

	matched := make(map[string]struct{})
	for k, g := range m.globs {
		for _, t := range tests {
			if g.MatchString(t) {
				matched[k] = struct{}{}
				break
			}
		}
	}
	for _, t := range tests {
		if _, ok := m.names[t]; ok {
			matched[t] = struct{}{}
		}
	}

	var notFoundList []string
	for k := range m.globs {
		if _, ok := matched[k]; !ok {
			notFoundList = append(notFoundList, k)
		}
	}
	for n := range m.names {
		if _, ok := matched[n]; !ok {
			notFoundList = append(notFoundList, n)
		}
	}

	sort.Strings(notFoundList)
	return notFoundList
}
