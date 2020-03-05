// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides the hardware dependency mechanism to select tests to run on
// a DUT based on its hardware features and setup.
package hwdep

import (
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing/internal/hwdep"
)

// Deps holds hardware dependencies all of which need to be satisfied to run a test.
type Deps = hwdep.Deps

// Condition represents one condition of hardware dependencies.
type Condition = hwdep.Condition

// D returns hardware dependencies representing the given Conditions.
func D(conds ...Condition) Deps {
	return hwdep.D(conds...)
}

// Model returns a hardware dependency condition that is satisfied if the DUT's model ID is
// one of the given names.
// Practically, this is not recommended to be used in most cases. Please consider again
// if this is the appropriate use, and whether there exists another option, such as
// check whether DUT needs to have touchscreen, some specific SKU, internal display etc.
//
// Expected example use case is; there is a problem in some code where we do not have
// control, such as a device specific driver, or hardware etc., and unfortnately
// it unlikely be fixed for a while.
// Another use case is; a test is stably running on most of models, but failing on some
// specific models. By using Model() and SkipOnModel() combination, the test can be
// promoted to critical on stably running models, while it is still informational
// on other models. Note that, in this case, it is expected that an engineer is
// assigned to stabilize/fix issues of the test on informational models.
func Model(names ...string) Condition {
	return func(d *hwdep.DeviceSetup) error {
		// If the field is unavailable, return false as not satisfied.
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.ModelId == nil {
			return errors.New("device.Config does not have ModelId")
		}
		modelID := d.DC.Id.ModelId.Value
		for _, name := range names {
			if name == modelID {
				return nil
			}
		}
		return errors.New("ModelId did not match")
	}
}

// SkipOnModel returns a hardware dependency condition that is satisfied
// iff the DUT's model ID is none of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnModel(names ...string) Condition {
	return func(d *hwdep.DeviceSetup) error {
		// If the field is unavailable, return false as not satisfied.
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.ModelId == nil {
			return errors.New("device.Config does not have ModelId")
		}
		modelID := d.DC.Id.ModelId.Value
		for _, name := range names {
			if name == modelID {
				return errors.New("ModelId matched with skip-on list")
			}
		}
		return nil
	}
}

// Platform returns a hardware dependency condition that is satisfied
// iff the DUT's platform ID is one of the give names.
// Please find the doc of Model(), too, for details about the expected usage.
func Platform(names ...string) Condition {
	return func(d *hwdep.DeviceSetup) error {
		// If the field is unavailable, return false as not satisfied.
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.PlatformId == nil {
			return errors.New("device.Config does not have PlatformId")
		}
		platformID := d.DC.Id.PlatformId.Value
		for _, name := range names {
			if name == platformID {
				return nil
			}
		}
		return errors.New("PlatformId did not match")
	}
}

// SkipOnPlatform returns a hardware dependency condition that is satisfied
// iff the DUT's platform ID is none of the give names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnPlatform(names ...string) Condition {
	return func(d *hwdep.DeviceSetup) error {
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.PlatformId == nil {
			return errors.New("device.Config does not have PlatformId")
		}
		platformID := d.DC.Id.PlatformId.Value
		for _, name := range names {
			if name == platformID {
				return errors.New("PlatformId matched with skip-on list")
			}
		}
		return nil
	}
}

// TouchScreen returns a hardware dependency condition that is satisfied
// iff the DUT has touchscreen.
func TouchScreen() Condition {
	return func(d *hwdep.DeviceSetup) error {
		if d.DC == nil {
			return errors.New("device.Config is not given")
		}
		for _, f := range d.DC.HardwareFeatures {
			if f == device.Config_HARDWARE_FEATURE_TOUCHSCREEN {
				return nil
			}
		}
		return errors.New("DUT does not have touchscreen")
	}
}
