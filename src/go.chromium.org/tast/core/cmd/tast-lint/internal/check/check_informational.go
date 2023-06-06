// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
)

const (
	shouldBeInformationalMsg = `Newly added tests should be marked as 'informational'.`
)

// VerifyInformationalAttr checks if a newly added test has 'informational' attribute.
func VerifyInformationalAttr(fs *token.FileSet, f *ast.File) []*Issue {
	return checkAttr(fs, f,
		func(attrs []string, pos token.Position) []*Issue {
			if isCriticalTest(attrs) {
				return []*Issue{{
					Pos:  pos,
					Msg:  shouldBeInformationalMsg,
					Link: testRegistrationURL,
				}}
			}
			return nil
		},
	)
}

// isCriticalTest returns true if there are no 'informational' attribute
// in existing attributes and it is mainline test.
func isCriticalTest(attrs []string) bool {
	isMainlineTest := false
	isInformational := false
	for _, attr := range attrs {
		if attr == "informational" {
			isInformational = true
		} else if attr == "group:mainline" {
			isMainlineTest = true
		}
	}
	return isMainlineTest && !isInformational
}
