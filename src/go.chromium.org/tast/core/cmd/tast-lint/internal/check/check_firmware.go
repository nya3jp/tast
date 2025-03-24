// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/exp/slices"
)

const (
	nonBiosRequirementMsg            = `No tests may include the requirements "sys-fw-0021-v01" and "sys-fw-0024-v01".`
	nonBiosRWRequirementMsg          = `No tests may include the requirement "sys-fw-0025-v01".`
	biosTestInvalidAttrsMsg          = `The attr "firmware_ro" can only be used with "firmware_bios_ro" and not with "firmware_bios_rw".`
	nonECRequirementMsg              = `No tests may include the requirement "sys-fw-0022-v02".`
	pdRequirementIndirectMsg         = `No tests may include the requirement "sys-fw-0023-v01".`
	missingGateAttr                  = `Tests for %[1]q must include %[2]q also.`
	secondaryAttrWithoutRequiredAttr = `Tests that use %[1]q must also include one of firmware_{ec,bios,pd} and group:firmware.`
	secondaryAttrWithoutBios         = `Tests that use firmware_bios_{rw,ro} must also include firmware_bios and group:firmware.`
	secondaryAttrWithoutECPD         = `Tests that use firmware_ec_{rw,ro} must also include firmware_{ec,pd} and group:firmware.`
	biosWithoutSecondaryAttr         = `Tests that use firmware_bios must also include one of firmware_{enabled,meets_kpi,stressed,bios_ro,bios_rw} and group:firmware.`
	biosRWWithoutRO                  = `Tests that use firmware_bios_rw must also include firmware_bios_ro and group:firmware.`
	ecRWWithoutRO                    = `Tests that use firmware_ec_rw must also include firmware_ec_ro and group:firmware.`
	ecWithoutSecondaryAttr           = `Tests that use firmware_ec must also include one of firmware_{enabled,meets_kpi,stressed,ec_ro,ec_rw} and group:firmware.`
	pdWithoutSecondaryAttr           = `Tests that use firmware_pd must also include one of firmware_{enabled,meets_kpi,stressed,ec_ro,ec_rw,bios_pdc} and group:firmware.`
	pdcWithoutPd                     = `Tests that use firmware_bios_pdc must also include firmware_pd and group:firmware.`
)

// VerifyFirmwareAttrs checks that "group:firmware" related attributes are set correctly.
func VerifyFirmwareAttrs(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue
	issues = append(issues, checkAttr(fs, f, firmwareNoRequirements)...)
	issues = append(issues, checkAttr(fs, f, firmwareSecondaryAttrChecker)...)
	return issues
}

func firmwareNoRequirements(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
	var issues []*Issue

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
	if slices.Contains(requirements, "sys-fw-0022-v02") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  nonECRequirementMsg,
			Link: testAttrDocURL,
		})
	}
	if slices.Contains(requirements, "sys-fw-0023-v01") {
		issues = append(issues, &Issue{
			Pos:  requirementPos,
			Msg:  pdRequirementIndirectMsg,
			Link: testAttrDocURL,
		})
	}

	return issues
}

func firmwareSecondaryAttrChecker(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
	var issues []*Issue

	hasFirmware := slices.Contains(attrs, "group:firmware")

	// These are ordered from earliest gate -> latest gate.
	phaseGates := []string{"firmware_enabled", "firmware_meets_kpi", "firmware_stressed"}

	// Any use of phase gate attrs or firmware qual attrs must also include one of these
	requiredAttrs := []string{"firmware_ec", "firmware_bios", "firmware_pd"}
	for i := 0; i < len(phaseGates)-1; i++ {
		if slices.Contains(attrs, phaseGates[i]) {
			if !slices.Contains(attrs, phaseGates[i+1]) {
				issues = append(issues, &Issue{
					Pos:  attrPos,
					Msg:  fmt.Sprintf(missingGateAttr, phaseGates[i], phaseGates[i+1]),
					Link: testAttrDocURL,
				})
			}
			hasReq := false
			for _, req := range requiredAttrs {
				if slices.Contains(attrs, req) {
					hasReq = true
					break
				}
			}
			if !hasReq || !hasFirmware {
				issues = append(issues, &Issue{
					Pos:  attrPos,
					Msg:  fmt.Sprintf(secondaryAttrWithoutRequiredAttr, phaseGates[i]),
					Link: testAttrDocURL,
				})
			}
		}
	}

	// Any bios, ec, or pd can be tagged with only phases or phases and other secondary attrs as appropriate.
	hasPhase := false
	for _, a := range phaseGates {
		if slices.Contains(attrs, a) {
			hasPhase = true
			break
		}
	}

	// All bios tests without phases need firmware_bios_ro
	if slices.Contains(attrs, "firmware_bios") && ((!hasPhase && !slices.Contains(attrs, "firmware_bios_ro")) || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosWithoutSecondaryAttr,
			Link: testAttrDocURL,
		})
	}
	// All with firmware_bios_rw need firmware_bios_ro
	if slices.Contains(attrs, "firmware_bios_rw") && (!slices.Contains(attrs, "firmware_bios_ro") || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosRWWithoutRO,
			Link: testAttrDocURL,
		})
	}
	// The firmware_ro attr is deprecated, but if it is present, then firmware_bios_ro must be also.
	if slices.Contains(attrs, "firmware_ro") && (!slices.Contains(attrs, "firmware_bios_ro") || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosTestInvalidAttrsMsg,
			Link: testAttrDocURL,
		})
	}
	// The firmware_ro attr is deprecated, but if it is present, firmware_bios_rw must not be.
	if slices.Contains(attrs, "firmware_ro") && slices.Contains(attrs, "firmware_bios_rw") {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  biosTestInvalidAttrsMsg,
			Link: testAttrDocURL,
		})
	}
	// firmware_bios_ro and firmware_bios_rw need firmware_bios
	if (slices.Contains(attrs, "firmware_bios_ro") || slices.Contains(attrs, "firmware_bios_rw")) && (!slices.Contains(attrs, "firmware_bios") || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  secondaryAttrWithoutBios,
			Link: testAttrDocURL,
		})
	}

	// firmware_bios_pdc should be used with firmware_pd, it is not intuitive, but the bios image carries the PDC firmware.
	// The bios tests don't do any testing of the PDC firmware though.
	if slices.Contains(attrs, "firmware_bios_pdc") && (!slices.Contains(attrs, "firmware_pd") || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  pdcWithoutPd,
			Link: testAttrDocURL,
		})
	}

	// All ec tests without phases need firmware_ec_ro
	if slices.Contains(attrs, "firmware_ec") && ((!hasPhase && !slices.Contains(attrs, "firmware_ec_ro")) || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  ecWithoutSecondaryAttr,
			Link: testAttrDocURL,
		})
	}
	// All with firmware_ec_rw need firmware_ec_ro
	if slices.Contains(attrs, "firmware_ec_rw") && (!slices.Contains(attrs, "firmware_ec_ro") || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  ecRWWithoutRO,
			Link: testAttrDocURL,
		})
	}
	// firmware_ec_ro and firmware_ec_rw need firmware_ec or firmware_pd
	if (slices.Contains(attrs, "firmware_ec_ro") || slices.Contains(attrs, "firmware_ec_rw")) && ((!slices.Contains(attrs, "firmware_ec") && !slices.Contains(attrs, "firmware_pd")) || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  secondaryAttrWithoutECPD,
			Link: testAttrDocURL,
		})
	}

	// All pd tests without phases need firmware_ec_ro or firmware_bios_pdc
	if slices.Contains(attrs, "firmware_pd") && ((!hasPhase && !slices.Contains(attrs, "firmware_ec_ro") && !slices.Contains(attrs, "firmware_bios_pdc")) || !hasFirmware) {
		issues = append(issues, &Issue{
			Pos:  attrPos,
			Msg:  pdWithoutSecondaryAttr,
			Link: testAttrDocURL,
		})
	}

	return issues
}
