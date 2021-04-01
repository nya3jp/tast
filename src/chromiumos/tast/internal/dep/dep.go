// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dep deals with dependencies of tests.
package dep

import (
	"fmt"
	"strings"

	"chromiumos/tast/errors"
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

// Check performs dependency checks according to given features.
// On success, it returns a list of reasons for which a test should be skipped.
// If reasons is empty, a test should be run.
func (d *Deps) Check(f *Features) (reasons []string, err error) {
	if f.Var != nil {
		for _, v := range d.Var {
			if _, ok := f.Var[v]; !ok {
				reasons = append(reasons, fmt.Sprintf("var %s not provided", v))
			}
		}
	}
	if f.Software != nil {
		missing, unknown := missingSoftwareDeps(d.Software, f.Software)
		if len(unknown) > 0 {
			return nil, errors.Errorf("unknown SoftwareDeps: %v", strings.Join(unknown, ", "))
		}
		if len(missing) > 0 {
			reasons = append(reasons, fmt.Sprintf("missing SoftwareDeps: %s", strings.Join(missing, ", ")))
		}
	}
	if f.Hardware != nil {
		if err := d.Hardware.Satisfied(f.Hardware); err != nil {
			for _, r := range err.Reasons {
				reasons = append(reasons, r.Error())
			}
		}
	}
	return reasons, nil
}
