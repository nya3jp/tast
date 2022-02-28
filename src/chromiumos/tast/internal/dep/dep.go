// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dep deals with dependencies of tests.
package dep

import (
	"fmt"
	"regexp"
	"strings"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// Deps contains all information about dependencies tests have.
type Deps struct {
	Var      []string
	Software map[string]SoftwareDeps
	Hardware map[string]HardwareDeps
}

// Check performs dependency checks according to given features.
// On success, it returns a list of reasons for which a test should be skipped.
// If reasons is empty, a test should be run.
func (d *Deps) Check(f *protocol.Features) (reasons []string, err error) {
	if !f.GetCheckDeps() {
		return nil, nil
	}

	// If f.MaybeMissingVars is empty, no variables are considered as missing.
	maybeMissingVars, err := regexp.Compile("^" + f.GetInfra().GetMaybeMissingVars() + "$")
	if err != nil {
		return nil, errors.Errorf("regex %v is invalid: %v", f.GetInfra().GetMaybeMissingVars(), err)
	}

	vars := f.GetInfra().GetVars()
	for _, v := range d.Var {
		if _, ok := vars[v]; ok {
			continue
		}
		if maybeMissingVars.MatchString(v) {
			reasons = append(reasons, fmt.Sprintf("runtime variable %v is missing and matches with %v", v, maybeMissingVars))
			continue
		}
		if f.GetInfra().GetMaybeMissingVars() == "" {
			return nil, errors.Errorf("runtime variable %v is missing", v)
		}
		return nil, errors.Errorf("runtime variable %v is missing and doesn't match with %v", v, maybeMissingVars)
	}

	for role, swDep := range d.Software {
		var dut *protocol.DUTFeatures
		if role != "" {
			var valid bool
			if dut, valid = f.GetCompanionFeatures()[role]; !valid {
				continue
			}
		} else {
			dut = f.GetDut()
		}

		missing, unknown := missingSoftwareDeps(swDep, dut.GetSoftware())

		if len(unknown) > 0 {
			return nil, errors.Errorf("unknown SoftwareDeps: %v", strings.Join(unknown, ", "))
		}
		if len(missing) > 0 {
			reasons = append(reasons, fmt.Sprintf("missing SoftwareDeps: %s", strings.Join(missing, ", ")))
		}
	}

	for role, hwDep := range d.Hardware {
		var dut *protocol.DUTFeatures
		if role != "" {
			var valid bool
			dut, valid = f.GetCompanionFeatures()[role]
			if !valid {
				continue
			}
		} else {
			dut = f.GetDut()
		}

		sat, err := hwDep.Satisfied(dut.GetHardware())
		if err != nil {
			return nil, err
		}
		for _, r := range sat {
			reasons = append(reasons, r)
		}
	}

	return reasons, nil
}
