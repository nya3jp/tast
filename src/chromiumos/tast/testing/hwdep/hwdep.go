// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides the hardware dependency mechanism to select tests to run on
// a DUT based on its hardware features and setup.
package hwdep

import (
	"fmt"
	"regexp"
	"strings"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/protocol"
)

// Deps holds hardware dependencies all of which need to be satisfied to run a test.
type Deps = dep.HardwareDeps

// Condition represents one condition of hardware dependencies.
type Condition = dep.HardwareCondition

// D returns hardware dependencies representing the given Conditions.
func D(conds ...Condition) Deps {
	return dep.NewHardwareDeps(conds...)
}

// idRegexp is the pattern that the given model/platform ID names should match with.
var idRegexp = regexp.MustCompile(`^[a-z0-9_-]+$`)

func satisfied() (bool, string, error) {
	return true, "", nil
}

func unsatisfied(reason string) (bool, string, error) {
	return false, reason, nil
}

func withError(err error) (bool, string, error) {
	return false, "", err
}

func withErrorStr(s string) (bool, string, error) {
	return false, "", errors.New(s)
}

// modelListed returns whether the model represented by a protocol.DeprecatedDeviceConfig is listed
// in the given list of names or not.
func modelListed(dc *protocol.DeprecatedDeviceConfig, names ...string) (bool, error) {
	if dc == nil || dc.Id == nil || dc.Id.Model == "" {
		return false, errors.New("DeprecatedDeviceConfig does not have model ID")
	}
	m := dc.Id.Model
	// Remove the suffix _signed since it is not a part of a model name.
	modelID := strings.TrimSuffix(strings.ToLower(m), "_signed")
	for _, name := range names {
		if name == modelID {
			return true, nil
		}
	}
	return false, nil
}

// platformListed returns whether the platform represented by a protocol.HardwareFeatures
// is listed in the given list of names or not.
func platformListed(dc *protocol.DeprecatedDeviceConfig, names ...string) (bool, error) {
	if dc == nil || dc.Id == nil {
		return false, errors.New("DeprecatedDeviceConfig does not have platform ID")
	}
	p := dc.Id.Platform
	platformID := strings.ToLower(p)
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
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := modelListed(f.GetDeprecatedDeviceConfig(), names...)
		// Currently amd64-generic vm does not return a model ID.
		// Treat it as a model that does not match any of the known ids.

		// TODO(b:190010549): Propagate the error once amd64-generic vm is handled properly.
		// if err != nil {
		//   	 return withError(err)
		// }

		if err != nil || !listed {
			return unsatisfied("ModelId did not match")
		}
		return satisfied()
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
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := modelListed(f.GetDeprecatedDeviceConfig(), names...)
		if err != nil {
			// Failed to get the model name.
			// Run the test to report error if it fails on this device.
			return satisfied()
		}
		if listed {
			return unsatisfied("ModelId matched with skip-on list")
		}
		return satisfied()
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
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := platformListed(f.GetDeprecatedDeviceConfig(), names...)
		if err != nil {
			return withError(err)
		}
		if !listed {
			return unsatisfied("PlatformId did not match")
		}
		return satisfied()
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
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := platformListed(f.GetDeprecatedDeviceConfig(), names...)
		if err != nil {
			return withError(err)
		}
		if listed {
			return unsatisfied("PlatformId matched with skip-on list")
		}
		return satisfied()
	}}
}

// TouchScreen returns a hardware dependency condition that is satisfied
// iff the DUT has touchscreen.
func TouchScreen() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		if hf.GetScreen().GetTouchSupport() == configpb.HardwareFeatures_PRESENT {
			return satisfied()
		}
		return unsatisfied("DUT does not have touchscreen")
	}, CEL: "dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT",
	}
}

// ChromeEC returns a hardware dependency condition that is satisfied
// iff the DUT has a present EC of the "Chrome EC" type.
func ChromeEC() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		ecIsPresent := hf.GetEmbeddedController().GetPresent() == configpb.HardwareFeatures_PRESENT
		ecIsChrome := hf.GetEmbeddedController().GetEcType() == configpb.HardwareFeatures_EmbeddedController_EC_CHROME
		if ecIsPresent && ecIsChrome {
			return satisfied()
		}
		return unsatisfied("DUT does not have chrome EC")
	}, CEL: "dut.hardware_features.embedded_controller.present == api.HardwareFeatures.Present.PRESENT && dut.hardware_features.embedded_controller.ec_type == api.HardwareFeatures.EmbeddedController.EmbeddedControllerType.EC_CHROME",
	}
}

// CPUSupportsSMT returns a hardware dependency condition that is satisfied iff the DUT supports
// Symmetric Multi-Threading.
func CPUSupportsSMT() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		for _, f := range hf.GetSoc().Features {
			if f == configpb.Component_Soc_SMT {
				return satisfied()
			}
		}
		return unsatisfied("CPU does not have SMT support")
	}, CEL: "dut.hardware_features.soc.features.exists(x, x == api.Component.Soc.Features.SMT)",
	}
}

// ECHibernate returns a hardware dependency condition that is satisfied
// iff the EC has the ability to hibernate.
func ECHibernate() Condition {
	names := []string{"fizz", "kukui", "puff", "scarlet"}
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := platformListed(f.GetDeprecatedDeviceConfig(), names...)
		if err != nil {
			return withError(err)
		}
		if listed {
			return unsatisfied("DUT does not support EC hibernate")
		}
		return satisfied()
	}}
}

// Fingerprint returns a hardware dependency condition that is satisfied
// iff the DUT has fingerprint sensor.
func Fingerprint() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetFingerprint().GetLocation() == configpb.HardwareFeatures_Fingerprint_NOT_PRESENT {
			return unsatisfied("DUT does not have fingerprint sensor")
		}
		return satisfied()
	}, CEL: "dut.hardware_features.fingerprint.location != api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT",
	}
}

// NoFingerprint returns a hardware dependency condition that is satisfied
// if the DUT doesn't have fingerprint sensor.
func NoFingerprint() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetFingerprint().GetLocation() != configpb.HardwareFeatures_Fingerprint_NOT_PRESENT {
			return unsatisfied("DUT has fingerprint sensor")
		}
		return satisfied()
	}, CEL: "dut.hardware_features.fingerprint.location == api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT",
	}
}

// InternalDisplay returns a hardware dependency condition that is satisfied
// iff the DUT has an internal display, e.g. Chromeboxes and Chromebits don't.
func InternalDisplay() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetScreen().GetPanelProperties() != nil {
			return satisfied()
		}
		return unsatisfied("DUT does not have an internal display")
	}, CEL: "dut.hardware_features.screen.panel_properties.diagonal_milliinch != 0",
	}
}

// Wifi80211ac returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ac.
func Wifi80211ac() Condition {
	// Some of guado and kip SKUs do not support 802.11ac.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	c := SkipOnPlatform("kip", "guado")
	c.CEL = "dut.hardware_features.wifi.supported_wlan_protocols.exists(x, x == api.Component.Wifi.WLANProtocol.IEEE_802_11_AC)"
	return c
}

// Wifi80211ax returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ax.
func Wifi80211ax() Condition {
	// Note: this is currently a blocklist.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		// TODO(crbug.com/1115620): remove "Elm" and "Hana" after unibuild migration
		// completed.
		platformCondition := SkipOnPlatform(
			"asuka",
			"asurada",
			"banjo",
			"banon",
			"bob",
			"buddy",
			"candy",
			"caroline",
			"cave",
			"celes",
			"chell",
			"coral",
			"cyan",
			"edgar",
			"enguarde",
			"eve",
			"fievel",
			"fizz",
			"gale",
			"gandof",
			"gnawty",
			"gru", // The mosys for scarlet is gru. scarlet does not support 802.11ax
			"grunt",
			"guado",
			"hana",
			"jecht", // The mosys for tidus is jecht. tidus does not support 802.11ax
			"kalista",
			"kefka",
			"kevin",
			"kip",
			"kukui", // The mosys for jacuzzi is kukui. jacuzzi does not support 802.11ax
			"lars",
			"lulu",
			"nami",
			"nautilus",
			"ninja",
			"nocturne",
			"oak", // The mosys for elm is oak. elm does not support 802.11ax
			"octopus",
			"orco",
			"paine",
			"poppy", // The mosys for atlas is poppy. atlas does not support 802.11ax
			"puff",
			"pyro",
			"rammus",
			"reef",
			"reks",
			"relm",
			"rikku",
			"samus",
			"sand",
			"sarien",
			"sentry",
			"setzer",
			"snappy",
			"soraka",
			"strongbad",
			"sumo",
			"swanky",
			"terra",
			"tiger",
			"trogdor",
			"trogdor-kernelnext",
			"ultima",
			"winky",
			"wizpig",
			"yuna")
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		// Some models of boards excluded from the platform skip do not support
		// 802.11ax. To be precise as possible, we will skip these models as well.
		modelCondition := SkipOnModel(
			"blipper",
			"dirinboz",
			"ezkinil",
			"gumboz",
			"jelboz",
			"jelboz360",
			"lantis",
			"madoo",
			"pirette",
			"pirika",
			"vilboz",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	}, CEL: "dut.hardware_features.wifi.supported_wlan_protocols.exists(x, x == api.Component.Wifi.WLANProtocol.IEEE_802_11_AX)",
	}
}

// WifiMACAddrRandomize returns a hardware dependency condition that is satisfied
// iff the DUT support WiFi MAC Address Randomization.
func WifiMACAddrRandomize() Condition {
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	c := SkipOnPlatform(
		// mwifiex in 3.10 kernel does not support it.
		"kitty",
		// Broadcom driver has only NL80211_FEATURE_SCHED_SCAN_RANDOM_MAC_ADDR
		// but not NL80211_FEATURE_SCAN_RANDOM_MAC_ADDR. We require randomization
		// for all supported scan types.
		"mickey", "minnie", "speedy",
	)
	c.CEL = "not_implemented"
	return c
}

// WifiNotMarvell returns a hardware dependency condition that is satisfied iff
// the DUT's not using a Marvell WiFi chip.
func WifiNotMarvell() Condition {
	// TODO(crbug.com/1070299): we don't yet have relevant fields in device.Config
	// about WiFi chip, so list the known platforms here for now.
	c := SkipOnPlatform(
		"bob", "kevin", "oak", "elm", "hana", "kitty",
		"mighty", "jaq", "fievel", "tiger", "jerry",
	)
	c.CEL = "not_implemented"
	return c
}

// WifiNotMarvell8997 returns a hardware dependency condition that is satisfied if
// the DUT is not using Marvell 8997 chipsets.
func WifiNotMarvell8997() Condition {
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	c := SkipOnPlatform(
		"bob", "kevin",
	)
	c.CEL = "not_implemented"
	return c
}

// WifiIntel returns a hardware dependency condition that if satisfied, indicates
// that a device uses Intel WiFi. It is not guaranteed that the condition will be
// satisfied for all devices with Intel WiFi.
func WifiIntel() Condition {
	// TODO(crbug.com/1070299): we don't yet have relevant fields in device.Config
	// about WiFi chip, so list the known platforms here for now.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		// TODO(crbug.com/1115620): remove "Elm" and "Hana" after unibuild migration
		// completed.
		// NB: Devices in the "scarlet" family use the platform name "gru", so
		// "gru" is being used here to represent "scarlet" devices.
		platformCondition := SkipOnPlatform(
			"asurada", "bob", "elm", "fievel", "gru", "grunt", "hana", "jacuzzi",
			"kevin", "kukui", "oak", "strongbad", "tiger", "trogdor", "trogdor-kernelnext",
		)
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		// NB: These exclusions are somewhat overly broad; for example, some
		// (but not all) blooglet devices have Intel WiFi chips. However,
		// for now there is no better way to specify the exact hardware
		// parameters needed for this dependency. (See crbug.com/1070299.)
		modelCondition := SkipOnModel(
			"blipper",
			"blooglet",
			"dirinboz",
			"ezkinil",
			"gumboz",
			"jelboz",
			"jelboz360",
			"lantis",
			"madoo",
			"pirette",
			"pirika",
			"vilboz",
			"vorticon",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	}, CEL: "not_implemented",
	}
}

// WifiQualcomm returns a hardware dependency condition that if satisfied, indicates
// that a device uses Qualcomm WiFi.
func WifiQualcomm() Condition {
	// TODO(crbug.com/1070299): we don't yet have relevant fields in device.Config
	// about WiFi chip, so list the known platforms here for now.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		platformCondition := Platform(
			"grunt", "kukui", "scarlet", "strongbad", "trogdor", "trogdor-kernelnext",
		)
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		// barla has Realtek WiFi chip.
		modelCondition := SkipOnModel(
			"barla",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	}, CEL: "not_implemented",
	}
}

func hasBattery(f *protocol.HardwareFeatures) (bool, error) {
	dc := f.GetDeprecatedDeviceConfig()
	if dc == nil {
		return false, errors.New("DeprecatedDeviceConfig is not given")
	}
	return dc.GetPower() == protocol.DeprecatedDeviceConfig_POWER_SUPPLY_BATTERY, nil
}

// Battery returns a hardware dependency condition that is satisfied iff the DUT
// has a battery, e.g. Chromeboxes and Chromebits don't.
func Battery() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hasBattery, err := hasBattery(f)
		if err != nil {
			return withError(err)
		}
		if !hasBattery {
			return unsatisfied("DUT does not have a battery")
		}
		return satisfied()
	}}
}

// SupportsNV12Overlays says true if the SoC supports NV12 hardware overlays,
// which are commonly used for video overlays. SoCs with Intel Gen 7.5 (Haswell,
// BayTrail) and Gen 8 GPUs (Broadwell, Braswell) for example, don't support
// those.
func SupportsNV12Overlays() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_HASWELL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_BAY_TRAIL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_BROADWELL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_BRASWELL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_U ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_Y ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_APOLLO_LAKE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_STONEY_RIDGE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8173 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8176 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8183 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8192 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8195 {
			return unsatisfied("SoC does not support NV12 Overlays")
		}
		return satisfied()
	}}
}

// Supports30bppFramebuffer says true if the SoC supports 30bpp color depth
// primary plane scanout. This is: Intel SOCs Kabylake and onwards, AMD SOCs
// from Zork onwards (codified Picasso), and not ARM SOCs.
func Supports30bppFramebuffer() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		// Any ARM CPUs
		if dc.GetCpu() == protocol.DeprecatedDeviceConfig_ARM ||
			dc.GetCpu() == protocol.DeprecatedDeviceConfig_ARM64 ||
			// Unknown SOCs
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED ||
			// Intel before Kabylake
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_APOLLO_LAKE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_BAY_TRAIL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_BRASWELL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_IVY_BRIDGE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_PINE_TRAIL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SANDY_BRIDGE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_BROADWELL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_HASWELL ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_U ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_Y ||
			// AMD before Zork
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_STONEY_RIDGE {
			return unsatisfied("SoC does not support scanning out 30bpp framebuffers")
		}
		return satisfied()
	}}
}

// Since there are no way to get whether an EC supports force discharging on a device or not,
// list up the models known not to support force discharging here.
var modelsWithoutForceDischargeSupport = []string{
	"arcada",
	"celes",
	"drallion",
	"drallion360",
	"lulu",
	"sarien",
}

// ForceDischarge returns a hardware dependency condition that is satisfied iff the DUT
// has a battery and it supports force discharge through `ectool chargecontrol`.
// The devices listed in modelsWithoutForceDischargeSupport do not satisfy this condition
// even though they have a battery since they does not support force discharge via ectool.
// This is a complementary condition of NoForceDischarge.
func ForceDischarge() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hasBattery, err := hasBattery(f)
		if err != nil {
			return withError(err)
		}
		if !hasBattery {
			return unsatisfied("DUT does not have a battery")
		}
		doesNotSupportForceDischarge, err := modelListed(f.GetDeprecatedDeviceConfig(), modelsWithoutForceDischargeSupport...)
		if err != nil {
			return withError(err)
		}
		if doesNotSupportForceDischarge {
			return unsatisfied("DUT has a battery but does not support force discharge")
		}
		return satisfied()
	}}
}

// NoForceDischarge is a complementary condition of ForceDischarge.
func NoForceDischarge() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		doesNotSupportForceDischarge, err := modelListed(f.GetDeprecatedDeviceConfig(), modelsWithoutForceDischargeSupport...)
		if err != nil {
			return withError(err)
		}
		if doesNotSupportForceDischarge {
			// Devices listed in modelsWithoutForceDischargeSupport
			// are known to always satisfy this condition
			return satisfied()
		}
		hasBattery, err := hasBattery(f)
		if err != nil {
			return withError(err)
		}
		if hasBattery {
			return unsatisfied("DUT supports force discharge")
		}
		return satisfied()
	}}
}

// X86 returns a hardware dependency condition matching x86 ABI compatible platform.
func X86() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86 || dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86_64 {
			return satisfied()
		}
		return unsatisfied("DUT's CPU is not x86 compatible")
	}}
}

// NoX86 returns a hardware dependency condition matching non-x86 ABI compatible platform.
func NoX86() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.GetCpu() != protocol.DeprecatedDeviceConfig_X86 && dc.GetCpu() != protocol.DeprecatedDeviceConfig_X86_64 {
			return satisfied()
		}
		return unsatisfied("DUT's CPU is x86 compatible")
	}}
}

// Nvme returns a hardware dependency condition if the device has an NVMe
// storage device.
func Nvme() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetStorage().GetStorageType() == configpb.Component_Storage_NVME {
			return satisfied()
		}
		return unsatisfied("DUT does not have an NVMe storage device")
	}, CEL: "dut.hardware_features.storage.storage_type == api.Component.Storage.StorageType.NVME",
	}
}

// MinStorage returns a hardware dependency condition requiring the minimum size of the storage in gigabytes.
func MinStorage(reqGigabytes int) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetStorage() == nil {
			return withErrorStr("Features.Storage was nil")
		}
		s := hf.GetStorage().GetSizeGb()
		if s < uint32(reqGigabytes) {
			return unsatisfied(fmt.Sprintf("The total storage size is smaller than required; got %dGB, need %dGB", s, reqGigabytes))
		}
		return satisfied()
	}, CEL: "not_implemented",
	}
}

// MinMemory returns a hardware dependency condition requiring the minimum size of the memory in megabytes.
func MinMemory(reqMegabytes int) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetMemory() == nil {
			return withErrorStr("Features.Memory was nil")
		}
		if hf.GetMemory().GetProfile() == nil {
			return withErrorStr("Features.Memory.Profile was nil")
		}
		s := hf.GetMemory().GetProfile().GetSizeMegabytes()
		if s < int32(reqMegabytes) {
			return unsatisfied(fmt.Sprintf("The total memory size is smaller than required; got %dMB, need %dMB", s, reqMegabytes))
		}
		return satisfied()
	}}
}

// MaxMemory returns a hardware dependency condition requiring no more than the
// maximum size of the memory in megabytes.
func MaxMemory(reqMegabytes int) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetMemory() == nil {
			return withErrorStr("Features.Memory was nil")
		}
		if hf.GetMemory().GetProfile() == nil {
			return withErrorStr("Features.Memory.Profile was nil")
		}
		s := hf.GetMemory().GetProfile().GetSizeMegabytes()
		if s > int32(reqMegabytes) {
			return unsatisfied(fmt.Sprintf("The total memory size is larger than required; got %dMB, need <= %dMB", s, reqMegabytes))
		}
		return satisfied()
	}}
}

// Speaker returns a hardware dependency condition that is satisfied iff the DUT has a speaker.
func Speaker() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetAudio().GetSpeakerAmplifier() != nil {
			return satisfied()
		}
		return unsatisfied("DUT does not have speaker")
	}, CEL: "has(dut.hardware_features.audio.speaker_amplifier)",
	}
}

// Microphone returns a hardware dependency condition that is satisfied iff the DUT has a microphone.
func Microphone() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetAudio().GetLidMicrophone().GetValue() > 0 || hf.GetAudio().GetBaseMicrophone().GetValue() > 0 {
			return satisfied()
		}
		return unsatisfied("DUT does not have microphone")
	}, CEL: "(dut.hardware_features.audio.lid_microphone.value > 0 || dut.hardware_features.audio.base_microphone.value > 0)",
	}
}

// PrivacyScreen returns a hardware dependency condition that is satisfied iff the DUT has a privacy screen.
func PrivacyScreen() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetPrivacyScreen().GetPresent() != configpb.HardwareFeatures_PRESENT {
			return unsatisfied("DUT does not have privacy screen")
		}
		return satisfied()
	}, CEL: "dut.hardware_features.privacy_screen.present == api.HardwareFeatures.Present.PRESENT",
	}
}
