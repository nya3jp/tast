// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides the hardware dependency mechanism to select tests to run on
// a DUT based on its hardware features and setup.
package hwdep

import (
	"regexp"
	"strings"

	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/testing/hwdep"
)

// Deps holds hardware dependencies all of which need to be satisfied to run a test.
type Deps = hwdep.Deps

// Condition represents one condition of hardware dependencies.
type Condition = hwdep.Condition

// D returns hardware dependencies representing the given Conditions.
func D(conds ...Condition) Deps {
	return hwdep.D(conds...)
}

// idRegexp is the pattern that the given model/plaform ID names should match with.
var idRegexp = regexp.MustCompile(`^[a-z0-9_]+$`)

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
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("ModelId should match with %v: %q", idRegexp, n)}
		}
	}

	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		// If the field is unavailable, return false as not satisfied.
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.ModelId == nil {
			return errors.New("device.Config does not have ModelId")
		}
		modelID := strings.ToLower(d.DC.Id.ModelId.Value)
		for _, name := range names {
			if name == modelID {
				return nil
			}
		}
		return errors.New("ModelId did not match")
	}}
}

// SkipOnModel returns a hardware dependency condition that is satisfied
// iff the DUT's model ID is none of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnModel(names ...string) Condition {
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("ModelId should match with %v: %q", idRegexp, n)}
		}
	}

	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		// If the field is unavailable, return false as not satisfied.
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.ModelId == nil {
			return errors.New("device.Config does not have ModelId")
		}
		modelID := strings.ToLower(d.DC.Id.ModelId.Value)
		for _, name := range names {
			if name == modelID {
				return errors.New("ModelId matched with skip-on list")
			}
		}
		return nil
	}}
}

// Platform returns a hardware dependency condition that is satisfied
// iff the DUT's platform ID is one of the give names.
// Please find the doc of Model(), too, for details about the expected usage.
func Platform(names ...string) Condition {
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("PlatformId should match with %v: %q", idRegexp, n)}
		}
	}

	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		// If the field is unavailable, return false as not satisfied.
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.PlatformId == nil {
			return errors.New("device.Config does not have PlatformId")
		}
		platformID := strings.ToLower(d.DC.Id.PlatformId.Value)
		for _, name := range names {
			if name == platformID {
				return nil
			}
		}
		return errors.New("PlatformId did not match")
	}}
}

// SkipOnPlatform returns a hardware dependency condition that is satisfied
// iff the DUT's platform ID is none of the give names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnPlatform(names ...string) Condition {
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("PlatformId should match with %v: %q", idRegexp, n)}
		}
	}

	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		if d.DC == nil || d.DC.Id == nil || d.DC.Id.PlatformId == nil {
			return errors.New("device.Config does not have PlatformId")
		}
		platformID := strings.ToLower(d.DC.Id.PlatformId.Value)
		for _, name := range names {
			if name == platformID {
				return errors.New("PlatformId matched with skip-on list")
			}
		}
		return nil
	}}
}

// TouchScreen returns a hardware dependency condition that is satisfied
// iff the DUT has touchscreen.
func TouchScreen() Condition {
	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		if d.DC == nil {
			return errors.New("device.Config is not given")
		}
		for _, f := range d.DC.HardwareFeatures {
			if f == device.Config_HARDWARE_FEATURE_TOUCHSCREEN {
				return nil
			}
		}
		return errors.New("DUT does not have touchscreen")
	}, CEL: "dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT",
	}
}

// Fingerprint returns a hardware dependency condition that is satisfied
// iff the DUT has fingerprint sensor.
func Fingerprint() Condition {
	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		if d.DC == nil {
			return errors.New("device.Config is not given")
		}
		for _, f := range d.DC.HardwareFeatures {
			if f == device.Config_HARDWARE_FEATURE_FINGERPRINT {
				return nil
			}
		}
		return errors.New("DUT does not have fingerprint sensor")
	}, CEL: "dut.hardware_features.fingerprint.location != api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT",
	}
}

// InternalDisplay returns a hardware dependency condition that is satisfied
// iff the DUT has an internal display, e.g. Chromeboxes and Chromebits don't.
func InternalDisplay() Condition {
	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		if d.DC == nil {
			return errors.New("device.Config is not given")
		}
		for _, f := range d.DC.HardwareFeatures {
			if f == device.Config_HARDWARE_FEATURE_INTERNAL_DISPLAY {
				return nil
			}
		}
		return errors.New("DUT does not have an internal display")
	}, CEL: "dut.hardware_features.screen.milliinch.value != 0U",
	}
}

// Wifi80211ac returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ac.
func Wifi80211ac() Condition {
	// Monroe (ath9k) does not support 802.11ac.
	// TODO(crbug.com/1024554): remove it after Monroe platform is end-of-life.
	// Some of guado and kip SKUs do not support 802.11ac.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return SkipOnPlatform("monroe", "kip", "guado")
}

// Battery returns a hardware dependency condition that is satisfied iff the DUT
// has a battery, e.g. Chromeboxes and Chromebits don't.
func Battery() Condition {
	return Condition{Satisfied: func(d *hwdep.DeviceSetup) error {
		if d.DC == nil {
			return errors.New("device.Config is not given")
		}
		if d.DC.Power != device.Config_POWER_SUPPLY_BATTERY {
			return errors.New("DUT does not have a battery")
		}
		return nil
	}}
}
