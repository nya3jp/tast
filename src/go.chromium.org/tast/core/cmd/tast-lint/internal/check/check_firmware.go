// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/exp/slices"
)

const (
	biosTestWithoutLevelMsg   = `All "firmware_bios" tests should also include a "firmware_level" attr.`
	biosTestMultipleLevelsMsg = `Only one "firmware_level" attr can be used on a test.`
	biosRequirementMsg        = `All "firmware_bios" tests should also include the requirements "sys-fw-0021-v01" and "sys-fw-0024-v01".`
	nonBiosRequirementMsg     = `Only "firmware_bios" tests may include the requirements "sys-fw-0021-v01" and "sys-fw-0024-v01".`
	biosRWRequirementMsg      = `All "firmware_bios" tests without "firmware_ro" should also include the requirement "sys-fw-0025-v01".`
	nonBiosRWRequirementMsg   = `Only "firmware_bios" tests without "firmware_ro" may include the requirement "sys-fw-0025-v01".`
	biosTestInvalidAttrsMsg   = `The attrs "firmware_level*" and "firmware_ro" can only be used with "firmware_bios".`
	ecRequirementMsg          = `All "firmware_ec" tests should also include the requirement "sys-fw-0022-v02".`
	nonECRequirementMsg       = `Only "firmware_ec" tests may include the requirement "sys-fw-0022-v02".`
	pdRequirementIndirectMsg  = `Don't use attr "firmware_pd" or requirement "sys-fw-0023-v01" directly, but instead use firmware.AddPDPorts().`
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
	if hasFirmware && hasBios && !slices.Contains(requirements, "sys-fw-0021-v01") && !slices.Contains(requirements, "sys-fw-0024-v01") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  biosRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	if !hasBios && (slices.Contains(requirements, "sys-fw-0021-v01") || slices.Contains(requirements, "sys-fw-0024-v01")) {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  nonBiosRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	if hasFirmware && hasBios && !hasRO && !slices.Contains(requirements, "sys-fw-0025-v01") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  biosRWRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	if (!hasBios || hasRO) && slices.Contains(requirements, "sys-fw-0025-v01") {
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

	hasFirmware := slices.Contains(attrs, "group:firmware")
	hasEC := slices.Contains(attrs, "firmware_ec")

	if hasFirmware && hasEC && !slices.Contains(requirements, "sys-fw-0022-v02") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  ecRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	if !hasEC && slices.Contains(requirements, "sys-fw-0022-v02") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  nonECRequirementMsg,
			Link: testAttrDocURL,
		})
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
	if slices.Contains(attrs, "firmware_pd") {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  pdRequirementIndirectMsg,
			Link: testAttrDocURL,
		})
	}
	return issues
}
