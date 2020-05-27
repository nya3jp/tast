// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dep deals with software/hardware dependencies of tests.
package dep

import (
	"fmt"
	"strings"
)

// Features contains information about all features of the DUT.
type Features struct {
	// Software contains information about software features.
	// If it is nil, software dependency checks should not be performed.
	Software *SoftwareFeatures

	// Hardware contains information about hardware features.
	// If it is nil, hardware dependency checks should not be performed.
	Hardware *HardwareFeatures
}

// Deps contains all information about all dependencies to features of the DUT.
type Deps struct {
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

// Check returns whether d is satisfied on the DUT having features f.
func (d *Deps) Check(f *Features) *CheckResult {
	var reasons []string
	var errs []string
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
