// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides the hardware dependency mechanism to select tests to run on
// a DUT based on its hardware features and setup.
package hwdep

import (
	"regexp"
	"strings"

	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
)

// Deps holds hardware dependencies all of which need to be satisfied to run a test.
type Deps = dep.HardwareDeps

// Condition represents one condition of hardware dependencies.
type Condition = dep.HardwareCondition

// D returns hardware dependencies representing the given Conditions.
func D(conds ...Condition) Deps {
	return dep.NewHardwareDeps(conds...)
}

// idRegexp is the pattern that the given model/plaform ID names should match with.
var idRegexp = regexp.MustCompile(`^[a-z0-9_]+$`)

// modelListed returns whether the model represented by a device.Config is listed in
// the given list of names or not.
func modelListed(dc *device.Config, names ...string) (bool, error) {
	if dc == nil || dc.Id == nil || dc.Id.ModelId == nil {
		return false, errors.New("device.Config does not have ModelId")
	}
	modelID := strings.ToLower(dc.Id.ModelId.Value)
	for _, name := range names {
		if name == modelID {
			return true, nil
		}
	}
	return false, nil
}

// platformListed returns whether the platform represented by a device.Config is listed in
// the given list of names or not.
func platformListed(dc *device.Config, names ...string) (bool, error) {
	if dc == nil || dc.Id == nil || dc.Id.PlatformId == nil {
		return false, errors.New("device.Config does not have PlatformId")
	}
	platformID := strings.ToLower(dc.Id.PlatformId.Value)
	for _, name := range names {
		if name == platformID {
			return true, nil
		}
	}
	return false, nil
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
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("ModelId should match with %v: %q", idRegexp, n)}
		}
	}
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		listed, err := modelListed(f.DC, names...)
		if err != nil {
			return err
		}
		if !listed {
			return errors.New("ModelId did not match")
		}
		return nil
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
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		listed, err := modelListed(f.DC, names...)
		if err != nil {
			return err
		}
		if listed {
			return errors.New("ModelId matched with skip-on list")
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
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		listed, err := platformListed(f.DC, names...)
		if err != nil {
			return err
		}
		if !listed {
			return errors.New("PlatformId did not match")
		}
		return nil
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
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		listed, err := platformListed(f.DC, names...)
		if err != nil {
			return err
		}
		if listed {
			return errors.New("PlatformId matched with skip-on list")
		}
		return nil
	}}
}

// TouchScreen returns a hardware dependency condition that is satisfied
// iff the DUT has touchscreen.
func TouchScreen() Condition {
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		if f.Features != nil {
			if f.Features.Screen.TouchSupport == configpb.HardwareFeatures_PRESENT {
				return nil
			}
			return errors.New("DUT does not have touchscreen")
		}

		// Kept for protocol compatibility with an older version of Tast command.
		// TODO(crbug.com/1094802): Remove this block when we bump sourceCompatVersion in tast/internal/build/compat.go.
		if f.DC != nil {
			for _, f := range f.DC.HardwareFeatures {
				if f == device.Config_HARDWARE_FEATURE_TOUCHSCREEN {
					return nil
				}
			}
			return errors.New("DUT does not have touchscreen")
		}
		return errors.New("DUT HardwareFeatures data is not given")
	}, CEL: "dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT",
	}
}

// Fingerprint returns a hardware dependency condition that is satisfied
// iff the DUT has fingerprint sensor.
func Fingerprint() Condition {
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		if f.Features != nil {
			if f.Features.Fingerprint.Location != configpb.HardwareFeatures_Fingerprint_NOT_PRESENT {
				return nil
			}
			return errors.New("DUT does not have fingerprint sensor")
		}
		if f.DC == nil {
			return errors.New("device.Config is not given")
		}
		for _, f := range f.DC.HardwareFeatures {
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
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		if f.Features != nil {
			if f.Features.Screen.PanelProperties != nil {
				return nil
			}
			return errors.New("DUT does not have an internal display")
		}
		if f.DC == nil {
			return errors.New("device.Config is not given")
		}
		for _, f := range f.DC.HardwareFeatures {
			if f == device.Config_HARDWARE_FEATURE_INTERNAL_DISPLAY {
				return nil
			}
		}
		return errors.New("DUT does not have an internal display")
	}, CEL: "dut.hardware_features.screen.panel_properties.diagonal_milliinch != 0",
	}
}

// Wifi80211ac returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ac.
func Wifi80211ac() Condition {
	// Some of guado and kip SKUs do not support 802.11ac.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return SkipOnPlatform("kip", "guado")
}

// Wifi80211ax returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ax.
func Wifi80211ax() Condition {
	// Note: this is currently an allowlist. We can consider switching this to a
	// blocklist/skiplist if we start adding too many relevant devices.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return Platform("hatch")
}

// WifiMACAddrRandomize returns a hardware dependency condition that is satisfied
// iff the DUT support WiFi MAC Address Randomization.
func WifiMACAddrRandomize() Condition {
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return SkipOnPlatform(
		// mwifiex in 3.10 kernel does not support it.
		"kitty",
		// Broadcom driver has only NL80211_FEATURE_SCHED_SCAN_RANDOM_MAC_ADDR
		// but not NL80211_FEATURE_SCAN_RANDOM_MAC_ADDR. We require randomization
		// for all supported scan types.
		"mickey", "minnie", "speedy",
	)
}

// WifiNotMarvell returns a hardware dependency condition that is satisfied iff
// the DUT's not using a Marvell WiFi chip.
func WifiNotMarvell() Condition {
	// TODO(crbug.com/1070299): we don't yet have relevant fields in device.Config
	// about WiFi chip, so list the known platforms here for now.
	return SkipOnPlatform(
		"bob", "kevin", "oak", "elm", "hana", "kitty",
		"mighty", "jaq", "fievel", "tiger", "jerry",
	)
}

func hasBattery(f *dep.HardwareFeatures) (bool, error) {
	if f.DC == nil {
		return false, errors.New("device.Config is not given")
	}
	return f.DC.Power == device.Config_POWER_SUPPLY_BATTERY, nil
}

// Battery returns a hardware dependency condition that is satisfied iff the DUT
// has a battery, e.g. Chromeboxes and Chromebits don't.
func Battery() Condition {
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		hasBattery, err := hasBattery(f)
		if err != nil {
			return err
		}
		if !hasBattery {
			return errors.New("DUT does not have a battery")
		}
		return nil
	}}
}

// SupportsNV12Overlays says true if the SoC supports NV12 hardware overlays,
// which are commonly used for video overlays. SoCs with Intel Gen 7.5 (Haswell,
// BayTrail) and Gen 8 GPUs (Broadwell, Braswell) for example, don't support
// those.
func SupportsNV12Overlays() Condition {
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		if f.DC == nil {
			return errors.New("device.Config is not given")
		}
		if f.DC.Soc == device.Config_SOC_HASWELL ||
			f.DC.Soc == device.Config_SOC_BAY_TRAIL ||
			f.DC.Soc == device.Config_SOC_BROADWELL ||
			f.DC.Soc == device.Config_SOC_BRASWELL ||
			f.DC.Soc == device.Config_SOC_SKYLAKE_U ||
			f.DC.Soc == device.Config_SOC_SKYLAKE_Y ||
			f.DC.Soc == device.Config_SOC_APOLLO_LAKE ||
			f.DC.Soc == device.Config_SOC_MT8173 ||
			f.DC.Soc == device.Config_SOC_MT8176 ||
			f.DC.Soc == device.Config_SOC_MT8183 ||
			f.DC.Soc == device.Config_SOC_UNSPECIFIED {
			return errors.New("SoC does not support NV12 Overlays")
		}
		return nil
	}}
}

// Since there are no way to get whether an EC supports force discharging on a device or not,
// list up the models known not to support force discharging here.
var modelsWithoutForceDischargeSupport = []string{
	"arcada_signed",
	"celes",
	"drallion",
	"drallion360",
	"lulu",
	"sarien_signed",
}

// ForceDischarge returns a hardware dependency condition that is satisfied iff the DUT
// has a battery and it supports force discharge through `ectool chargecontrol`.
// The devices listed in modelsWithoutForceDischargeSupport do not satisfy this condition
// even though they have a battery since they does not support force discharge via ectool.
// This is a complementary condition of NoForceDischarge.
func ForceDischarge() Condition {
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		hasBattery, err := hasBattery(f)
		if err != nil {
			return err
		}
		if !hasBattery {
			return errors.New("DUT does not have a battery")
		}
		doesNotSupportForceDischarge, err := modelListed(f.DC, modelsWithoutForceDischargeSupport...)
		if err != nil {
			return err
		}
		if doesNotSupportForceDischarge {
			return errors.New("DUT has a battery but does not support force discharge")
		}
		return nil
	}}
}

// NoForceDischarge is a complementary condition of ForceDischarge.
func NoForceDischarge() Condition {
	return Condition{Satisfied: func(f *dep.HardwareFeatures) error {
		doesNotSupportForceDischarge, err := modelListed(f.DC, modelsWithoutForceDischargeSupport...)
		if err != nil {
			return err
		}
		if doesNotSupportForceDischarge {
			// Devices listed in modelsWithoutForceDischargeSupport
			// are known to always satisfy this condition
			return nil
		}
		hasBattery, err := hasBattery(f)
		if err != nil {
			return err
		}
		if hasBattery {
			return errors.New("DUT supports force discharge")
		}
		return nil
	}}
}
