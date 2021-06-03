// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"strings"

	"chromiumos/tast/internal/protocol"
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
	// Satisfied is a pointer to a function which checks if the given HardwareFeatures satisfies
	// the condition.
	Satisfied func(f *protocol.HardwareFeatures) (satisifed bool, reason string, err error)

	// CEL is the CEL expression denoting the condition.
	CEL string

	// Err is an error to be reported on Test registration
	// if instantiation of HardwareCondition fails.
	Err error
}

// NewHardwareDeps creates a HardwareDeps from a set of HWConditions.
func NewHardwareDeps(conds ...HardwareCondition) HardwareDeps {
	return HardwareDeps{conds: conds}
}

// UnsatisfiedReasons contain the detailed reasons why Satisfied failed. An empty list means success.
type UnsatisfiedReasons []string

// Satisfied returns whether the condition is satisfied.
// UnsatisfiedReasons is empty if the given device.Config satisfies the dependencies.
// i.e., the test can run on the current device setup.
// A non-nil error is returned when failed to evaluate the condition.
// Otherwise, the UnsatisfiedReasons instance contains a collection of reasons why any of the condition was not satsfied.
func (d *HardwareDeps) Satisfied(f *protocol.HardwareFeatures) (UnsatisfiedReasons, error) {
	var reasons UnsatisfiedReasons
	for _, c := range d.conds {
		if c.Satisfied == nil {
			reasons = append(reasons, "Satisfied was nil")
			continue
		}
		satisfied, reason, err := c.Satisfied(f)
		if err != nil {
			return nil, err
		}
		if !satisfied {
			if reason == "" {
				reason = "(no reason given)"
			}
			reasons = append(reasons, reason)
		}
	}
	return reasons, nil
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
