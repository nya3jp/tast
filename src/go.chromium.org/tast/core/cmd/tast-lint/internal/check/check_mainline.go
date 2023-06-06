// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"

	"golang.org/x/exp/slices"
)

const (
	informationalWithoutMainlineMsg        = `Attr "informational" should be used with "group:mainline".`
	criticalstagingWithoutInformationalMsg = `Attr "group:criticalstaging" should be used with "group:mainline" and "informational".`
	informationalWithGroupMsg              = `Attr "group:informational" is a typo of "informational".`
	criticalStagingWithoutGroupMsg         = `Attr "criticalstaging" is a typo of "group:criticalstaging".`

	attrDocURL = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md"
)

// VerifyMainlineAttrs checks that "group:mainline" related attributes are set correctly.
func VerifyMainlineAttrs(fs *token.FileSet, f *ast.File) []*Issue {
	return checkAttr(fs, f, mainlineAttrsChecker)
}

func mainlineAttrsChecker(attrs []string, pos token.Position) []*Issue {
	var issues []*Issue

	hasMainline := slices.Contains(attrs, "group:mainline")
	hasInformational := slices.Contains(attrs, "informational")
	hasCriticalStaging := slices.Contains(attrs, "group:criticalstaging")

	// Find typos.
	if slices.Contains(attrs, "group:informational") {
		issues = append(issues, &Issue{
			Pos:  pos,
			Msg:  informationalWithGroupMsg,
			Link: attrDocURL,
		})
		// Assume the intention is to set the correct Attr.
		hasInformational = true
	}
	if slices.Contains(attrs, "criticalstaging") {
		issues = append(issues, &Issue{
			Pos:  pos,
			Msg:  criticalStagingWithoutGroupMsg,
			Link: attrDocURL,
		})
		// Assume the intention is to set the correct Attr.
		hasCriticalStaging = true
	}

	if hasInformational && !hasMainline {
		issues = append(issues, &Issue{
			Pos:  pos,
			Msg:  informationalWithoutMainlineMsg,
			Link: attrDocURL,
		})
	}
	if hasCriticalStaging && !(hasMainline && hasInformational) {
		issues = append(issues, &Issue{
			Pos:  pos,
			Msg:  criticalstagingWithoutInformationalMsg,
			Link: attrDocURL,
		})
	}

	return issues
}
