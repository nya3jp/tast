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
	vmStableWithoutMainlineMsg      = `Attr "vm_stable" needs to be used with "group:mainline".`
	vmStableWithoutInformationalMsg = `Attr "vm_stable" needs to be used with "informational". 
									   If test is promoted to critical, remove the vm_stable attribute".`
	vmStableWithoutHWAgnosticMsg = `Attr "vm_stable" needs to be used with "hw_agnostic".`
)

// VerifyVMStableAttrs checks that "vm_stable" related attributes are set correctly.
func VerifyVMStableAttrs(fs *token.FileSet, f *ast.File) []*Issue {
	return checkAttr(fs, f, vmStableAttrChecker)
}

func vmStableAttrChecker(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
	var issues []*Issue

	hasMainline := slices.Contains(attrs, "group:mainline")
	hasInformational := slices.Contains(attrs, "informational")
	hasVMStable := slices.Contains(attrs, "hw_agnostic_vm_stable")
	hasHWAgnostic := slices.Contains(attrs, "group:hw_agnostic")

	// vm_stable attribute is to allow test owners to have automated test coverage (eg. CQ) for the
	// case where tests are stable for VMs, but not yet for HW runs. Tests are expected to be stabilized
	// by owners such that both vm_stable and informational attributes are removed --> promoted to critical.

	// Expected usage: group:mainline && group:hw_agnostic && informational && vm_stable

	// Missing hw_agnostic
	if hasVMStable && !hasHWAgnostic {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  vmStableWithoutHWAgnosticMsg,
			Link: testAttrDocURL,
		})
	}

	// Missing mainline
	if hasVMStable && !hasMainline {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  vmStableWithoutMainlineMsg,
			Link: testAttrDocURL,
		})
	}

	// Missing informational
	if hasVMStable && !hasInformational {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  vmStableWithoutInformationalMsg,
			Link: testAttrDocURL,
		})
	}

	return issues
}
