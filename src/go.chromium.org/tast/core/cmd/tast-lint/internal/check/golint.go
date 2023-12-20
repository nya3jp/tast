// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"strings"

	"golang.org/x/lint"
)

// minConfidence is the confidence threshold for problems reported from Golint.
// Golint's default is 0.8.
const minConfidence = 0.79

// shouldIgnore is called to filter irrelevant issues reported by Golint.
func shouldIgnore(p lint.Problem) bool {
	if p.Confidence < minConfidence {
		return true
	}

	// Ignore unexported-type-in-api.
	if p.Category == "unexported-type-in-api" {
		return true
	}

	// Tast test functions can be exported without comment.
	if isEntryFile(p.Position.Filename) &&
		p.Category == "comments" &&
		strings.Contains(p.Text, "should have comment or be unexported") {
		return true
	}

	if p.Category == "naming" {
		// Accept Id as an alternative of ID because protobuf generates Id
		// instead of ID for go bindings.
		check := func() bool {
			tokens := strings.SplitN(p.Text, " should be ", 2)
			if len(tokens) != 2 {
				return false
			}
			// Checks for functions, methods and their parameters.
			return (strings.HasPrefix(tokens[0], "func") || strings.HasPrefix(tokens[0], "method")) &&
				strings.Contains(tokens[0], "Id") &&
				strings.Contains(tokens[1], "ID")
		}()
		if check {
			return true
		}

	}

	return false
}

// Golint runs Golint to find issues.
func Golint(path string, code []byte, debug bool) []*Issue {
	ps, err := (&lint.Linter{}).Lint(path, code)
	if err != nil {
		panic(err)
	}

	var issues []*Issue
	for _, p := range ps {
		if shouldIgnore(p) {
			continue
		}

		var msg string
		if debug {
			msg = fmt.Sprintf("[%s; %.2f] %s", p.Category, p.Confidence, p.Text)
		} else {
			msg = p.Text
		}
		issues = append(issues, &Issue{
			Pos:  p.Position,
			Msg:  msg,
			Link: p.Link,
		})
	}
	return issues
}
