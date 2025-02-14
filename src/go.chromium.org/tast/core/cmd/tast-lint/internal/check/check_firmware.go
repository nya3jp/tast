// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/exp/slices"
)

const (
	biosTestWithoutLevelMsg   = `All "firmware_bios" tests should also include a "firmware_level" attr.`
	biosTestMultipleLevelsMsg = `Only one "firmware_level" attr can be used on a test.`
	nonBiosRequirementMsg     = `No tests may include the requirements "sys-fw-0021-v01" and "sys-fw-0024-v01".`
	nonBiosRWRequirementMsg   = `No tests may include the requirement "sys-fw-0025-v01".`
	biosTestInvalidAttrsMsg   = `The attrs "firmware_level*" and "firmware_ro" can only be used with "firmware_bios".`
	nonECRequirementMsg       = `No tests may include the requirement "sys-fw-0022-v02".`
	pdRequirementIndirectMsg  = `No tests may include the requirement "sys-fw-0023-v01".`
	missingGateAttr           = `Tests for %[1]q must include %[2]q also.`
)

// VerifyFirmwareAttrs checks that "group:firmware" related attributes are set correctly.
func VerifyFirmwareAttrs(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue
	issues = append(issues, checkAttr(fs, f, firmwareBiosAttrsChecker)...)
	issues = append(issues, checkAttr(fs, f, firmwareECAttrsChecker)...)
	issues = append(issues, checkAttr(fs, f, firmwarePDAttrsChecker)...)
	return issues
}

func firmwareBiosAttrsChecker(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
	var issues []*Issue

	hasFirmware := slices.Contains(attrs, "group:firmware")
	hasBios := slices.Contains(attrs, "firmware_bios")
	hasRO := slices.Contains(attrs, "firmware_ro")
	hasLevels := 0
	for _, a := range attrs {
		if strings.HasPrefix(a, "firmware_level") {
			hasLevels++
		}
	}

	if hasFirmware && hasBios && hasLevels < 1 {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosTestWithoutLevelMsg,
			Link: testAttrDocURL,
		})
	}
	if hasFirmware && hasBios && hasLevels > 1 {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosTestMultipleLevelsMsg,
			Link: testAttrDocURL,
		})
	}
	if (!hasFirmware || !hasBios) && (hasRO || hasLevels > 0) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosTestInvalidAttrsMsg,
			Link: testAttrDocURL,
		})
	}
	if slices.Contains(requirements, "sys-fw-0021-v01") || slices.Contains(requirements, "sys-fw-0024-v01") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  nonBiosRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	if slices.Contains(requirements, "sys-fw-0025-v01") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  nonBiosRWRequirementMsg,
			Link: testAttrDocURL,
		})
	}

	return issues
}

func firmwareECAttrsChecker(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
	var issues []*Issue

	if slices.Contains(requirements, "sys-fw-0022-v02") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  nonECRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	// These are ordered from earliest gate -> latest gate.
	phaseGates := []string{"firmware_ec_enabled", "firmware_ec_meets_kpi", "firmware_ec_stressed"}
	for i := 0; i < len(phaseGates)-1; i++ {
		if slices.Contains(attrs, phaseGates[i]) && !slices.Contains(attrs, phaseGates[i+1]) {
			issues = append(issues, &Issue{
				Pos:  attrPos,
				Msg:  fmt.Sprintf(missingGateAttr, phaseGates[i], phaseGates[i+1]),
				Link: testAttrDocURL,
			})
		}
	}

	return issues
}

func firmwarePDAttrsChecker(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
	var issues []*Issue

	if slices.Contains(requirements, "sys-fw-0023-v01") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  pdRequirementIndirectMsg,
			Link: testAttrDocURL,
		})
	}
	return issues
}
