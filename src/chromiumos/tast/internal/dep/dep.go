// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dep deals with dependencies of tests.
package dep

import (
	"fmt"
	"strings"
)

// Features conveys actual values test dependencies are checked against.
// TODO(oka): rename the struct name.
type Features struct {
	// Var contains runtime variables.
	// If it is nil, variable dependency checks should not be performed.
	Var map[string]string

	// Software contains information about software features.
	// If it is nil, software dependency checks should not be performed.
	Software *SoftwareFeatures

	// Hardware contains information about hardware features.
	// If it is nil, hardware dependency checks should not be performed.
	Hardware *HardwareFeatures
}

// Deps contains all information about dependencies tests have.
type Deps struct {
	Var      []string
	Software SoftwareDeps
	Hardware HardwareDeps
}

// CheckResult represents the result of the check whether to run a test.
type CheckResult struct {
	// SkipReasons contains a list of messages describing why some dependencies
	// were not satisfied. They should be reported as informational logs.
	SkipReasons []string

	// Errors contains a list of messages describing errors encountered while
	// evaluating dependencies. They should be reported as test errors.
	Errors []string
}

// OK returns whether to run the test.
func (r *CheckResult) OK() bool {
	return len(r.SkipReasons) == 0 && len(r.Errors) == 0
}

// Check returns whether d is satisfied on f.
func (d *Deps) Check(f *Features) *CheckResult {
	var reasons []string
	var errs []string
	if f.Var != nil {
		for _, v := range d.Var {
			if _, ok := f.Var[v]; !ok {
				reasons = append(reasons, fmt.Sprintf("var %s not provided", v))
			}
		}
	}
	if f.Software != nil {
		missing, unknown := missingSoftwareDeps(d.Software, f.Software)
		if len(missing) > 0 {
			reasons = append(reasons, fmt.Sprintf("missing SoftwareDeps: %s", strings.Join(missing, ", ")))
		}
		if len(unknown) > 0 {
			errs = append(errs, fmt.Sprintf("unknown SoftwareDeps: %v", strings.Join(unknown, ", ")))
		}
	}
	if f.Hardware != nil {
		if err := d.Hardware.Satisfied(f.Hardware); err != nil {
			for _, r := range err.Reasons {
				reasons = append(reasons, r.Error())
			}
		}
	}
	return &CheckResult{SkipReasons: reasons, Errors: errs}
}
