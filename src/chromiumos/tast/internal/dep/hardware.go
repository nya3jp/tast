// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"strings"

	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
)

// HardwareDeps is exported as chromiumos/tast/testing/hwdep.Deps. Please find its document for details.
type HardwareDeps struct {
	// conds hold a slice of HardwareConditions. The enclosing HardwareDeps instance will be satisfied
	// iff all conds return true. Note that, if conds is empty, HardwareDeps is considered as
	// satisfied.
	conds []HardwareCondition
}

// HardwareCondition is exported as chromiumos/tast/testing/hwdep.Condition. Please find its document for details.
// Either Satisfied or Err should be nil exclusively.
type HardwareCondition struct {
	// Satisfied is a pointer to a function which checks if the given DeviceSetup satisfies
	// the condition.
	Satisfied func(d *DeviceSetup) error

	// CEL is the CEL expression denoting the condition.
	CEL string

	// Err is an error to be reported on Test registration
	// if instantiation of HardwareCondition fails.
	Err error
}

// DeviceSetup represents the configuration of the current DUT.
// Each condition expects to be implemented based only on this information.
type DeviceSetup struct {
	DC *device.Config
	// TODO(hidehiko): Consider adding lab peripherals here.
}

// NewHardwareDeps creates a HardwareDeps from a set of HWConditions.
func NewHardwareDeps(conds ...HardwareCondition) HardwareDeps {
	return HardwareDeps{conds: conds}
}

// UnsatisfiedError is reported when Satisfied() fails.
type UnsatisfiedError struct {
	*errors.E

	// Reasons contain the detailed reasons why Satisfied failed.
	Reasons []error
}

// Satisfied returns nil if the given device.Config satisfies the dependencies,
// i.e., the test can run on the current device setup.
// Otherwise, this returns an UnsatisfiedError instance, which contains a
// collection of detailed errors in Reasons.
func (d *HardwareDeps) Satisfied(dc *device.Config) *UnsatisfiedError {
	setup := &DeviceSetup{DC: dc}
	var reasons []error
	for _, c := range d.conds {
		if c.Satisfied == nil {
			reasons = append(reasons, errors.New("Satisfied was nil"))
			continue
		}
		if err := c.Satisfied(setup); err != nil {
			reasons = append(reasons, err)
		}
	}
	if len(reasons) > 0 {
		return &UnsatisfiedError{
			E:       errors.New("HardwareDeps is not satisfied"),
			Reasons: reasons,
		}
	}
	return nil
}

// Validate returns error if one of the conditions failed to be instantiated.
func (d *HardwareDeps) Validate() error {
	for _, c := range d.conds {
		if c.Err != nil {
			return c.Err
		}
	}
	return nil
}

// CEL returns the CEL expression that reflects the conditions.
func (d *HardwareDeps) CEL() string {
	var allConds []string
	for _, c := range d.conds {
		if c.CEL == "" {
			// Condition not supported by infra.
			// For example, some tests are made to skip on a certain board,
			// but those should be implemented as a more generic device feature.

			// Tentative placeholder to make the evaluation fail in infra.
			allConds = append(allConds, "not_implemented")
			continue
		}
		allConds = append(allConds, c.CEL)
	}
	return strings.Join(allConds, " && ")
}

// MergeHardwareDeps merges two HardwareDeps instance into one HardwareDeps instance.
// The returned HardwareDeps is satisfied iff all conditions in d1 and ones in d2 are
// satisfied.
func MergeHardwareDeps(d1, d2 HardwareDeps) HardwareDeps {
	var conds []HardwareCondition
	conds = append(conds, d1.conds...)
	conds = append(conds, d2.conds...)
	return HardwareDeps{conds: conds}
}
