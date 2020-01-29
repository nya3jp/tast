// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides implementation details of hardware dependency.
// Specifically, this is designed to be called only from framework, or
// exposed to tests via chromiumos/tast/testing/hwdep package.
package hwdep

import (
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
)

// Deps is exported via chromiumos/tast/testing/hwdep. Please find its document for details.
type Deps struct {
	// conds hold a slice of Conditions. The enclosing Deps instance will be satisfied
	// iff all conds return true. Note that, if conds is empty, Deps is considered as
	// satisfied.
	conds []Condition
}

// Condition is exported via chromiumos/tast/testing/hwdep. Please find its document for details.
type Condition func(d *DeviceSetup) error

// DeviceSetup represents the configuration of the current DUT.
// Each condition expects to be implemented based only on this information.
type DeviceSetup struct {
	DC *device.Config
	// TODO(hidehiko): Consider adding lab peripherals here.
}

// D is exported via chromiumos/tast/testing/hwdep. Please find its document for details
func D(conds ...Condition) Deps {
	return Deps{conds: conds}
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
func (d *Deps) Satisfied(dc *device.Config) *UnsatisfiedError {
	setup := &DeviceSetup{DC: dc}
	var reasons []error
	for _, c := range d.conds {
		if err := c(setup); err != nil {
			reasons = append(reasons, err)
		}
	}
	if len(reasons) > 0 {
		return &UnsatisfiedError{
			E:       errors.New("Deps is not satisfied"),
			Reasons: reasons,
		}
	}
	return nil
}

// Merge merges two Deps instance into one Deps instance.
// The returned Deps is satisfied iff all conditions in d1 and ones in d2 are
// satisfied.
func Merge(d1 Deps, d2 Deps) Deps {
	var conds []Condition
	conds = append(conds, d1.conds...)
	conds = append(conds, d2.conds...)
	return Deps{conds: conds}
}
