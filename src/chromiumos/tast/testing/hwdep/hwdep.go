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

// These are form factor values that can be passed to FormFactor and SkipOnFormFactor.
const (
	FormFactorUnknown = configpb.HardwareFeatures_FormFactor_FORM_FACTOR_UNKNOWN
	Clamshell         = configpb.HardwareFeatures_FormFactor_CLAMSHELL
	Convertible       = configpb.HardwareFeatures_FormFactor_CONVERTIBLE
	Detachable        = configpb.HardwareFeatures_FormFactor_DETACHABLE
	Chromebase        = configpb.HardwareFeatures_FormFactor_CHROMEBASE
	Chromebox         = configpb.HardwareFeatures_FormFactor_CHROMEBOX
	Chromebit         = configpb.HardwareFeatures_FormFactor_CHROMEBIT
	Chromeslate       = configpb.HardwareFeatures_FormFactor_CHROMESLATE
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
// Deprecated. Use Model() or "board:*" software dependency.
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
// Deprecated. Use SkipOnModel() or "board:*" software dependency.
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
	},
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
	},
	}
}

// ECFeatureTypecCmd returns a hardware dependency condition that is satisfied
// iff the DUT has an EC which supports the EC_FEATURE_TYPEC_CMD feature flag.
func ECFeatureTypecCmd() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		// We only return unsatisfied if we know for sure that the EC doesn't support the feature flag.
		// In cases where the result is UNKNOWN, we allow the test to continue and fail.
		if hf.GetEmbeddedController().GetFeatureTypecCmd() == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT EC does not support EC_FEATURE_TYPEC_CMD")
		}
		return satisfied()
	},
	}
}

// ECFeatureCBI returns a hardware dependency condition that
// is satisfied iff the DUT has an EC which supports CBI.
func ECFeatureCBI() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetEmbeddedController().GetCbi(); status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have cbi")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine cbi presence")
		}
		return satisfied()
	},
	}
}

// GSCUART returns a hardware dependency condition that is satisfied iff the DUT has a GSC and that GSC has a working UART.
// TODO(b/224608005): Add a cros_config for this and use that instead.
func GSCUART() Condition {
	// There is no way to probe for this condition, and there should be no machines newer than 2017 without working UARTs.
	return SkipOnModel(
		"astronaut",
		"blacktiplte",
		"caroline",
		"celes",
		"electro",
		"elm",
		"eve",
		"hana",
		"kefka",
		"lars",
		"nasher",
		"nocturne",
		"relm",
		"robo360",
		"sand",
		"sentry",
		"snappy",
	)
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
	},
	}
}

// CPUSupportsSHANI returns a hardware dependency condition that is satisfied iff the DUT supports
// SHA-NI instruction extension.
func CPUSupportsSHANI() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		for _, f := range hf.GetSoc().Features {
			if f == configpb.Component_Soc_SHA_NI {
				return satisfied()
			}
		}
		return unsatisfied("CPU does not have SHA-NI support")
	},
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
	},
	}
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
	},
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
	},
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
	},
	}
}

// NoInternalDisplay returns a hardware dependency condition that is satisfied
// iff the DUT does not have an internal display.
func NoInternalDisplay() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetScreen().GetPanelProperties() != nil {
			return unsatisfied("DUT has an internal display")
		}
		return satisfied()
	},
	}
}

// Keyboard returns a hardware dependency condition that is satisfied
// iff the DUT has an keyboard, e.g. Chromeboxes and Chromebits don't.
// Tablets might have a removable keyboard.
func Keyboard() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetKeyboard() == nil ||
			hf.GetKeyboard().KeyboardType == configpb.HardwareFeatures_Keyboard_KEYBOARD_TYPE_UNKNOWN ||
			hf.GetKeyboard().KeyboardType == configpb.HardwareFeatures_Keyboard_NONE {
			return unsatisfied("DUT does not have a keyboard")
		}
		return satisfied()
	},
	}
}

// KeyboardBacklight returns a hardware dependency condition that is satified
// if the DUT supports keyboard backlight functionality.
func KeyboardBacklight() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetKeyboard().GetBacklight() != configpb.HardwareFeatures_PRESENT {
			return unsatisfied("DUT does not have keyboard backlight")
		}
		return satisfied()
	},
	}
}

// Touchpad returns a hardware dependency condition that is satisfied
// iff the DUT has a touchpad.
func Touchpad() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTouchpad().GetPresent() != configpb.HardwareFeatures_PRESENT {
			return unsatisfied("DUT does not have a touchpad")
		}
		return satisfied()
	},
	}
}

// Wifi80211ac returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ac.
func Wifi80211ac() Condition {
	// Some of guado and kip SKUs do not support 802.11ac.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return SkipOnPlatform("kip", "guado")
}

// WifiWEP returns a hardware dependency condition that is satisfied
// if the DUT's WiFi module supports WEP.
// New generation of Qcom chipsets do not support WEP security protocols.
func WifiWEP() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		platformCondition := SkipOnPlatform(
			"herobrine")
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}

		modelCondition := SkipOnModel(
			"nipperkin")
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	},
	}
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
			"bob", // bob, kevin use the platform name "gru", they do need to be added to SkipOnModel
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
			"kevin64",
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
			"beetley",
			"blipper",
			"dirinboz",
			"ezkinil",
			"gooey",
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
	},
	}
}

// Wifi80211ax6E returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports WiFi 6E.
func Wifi80211ax6E() Condition {
	// Note: this is currently an allowlist. We can move this to a blocklist if the number of platforms gets out of hand.
	// TODO(crbug.com/1070299): replace this when we have hwdep for WiFi chips.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		modelCondition := Model(
			"anahera",
			"brya",
			"felwinter",
			"gimble",
			"herobrine",
			"kano",
			"nipperkin",
			"primus",
			"redrix",
			"taeko",
			"taeland",
			"vell",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	},
	}
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
	// TODO(b/187699768): we don't yet have relevant fields in device.Config
	// about WiFi chip, so list the known platforms here for now.
	// TODO(b/187699664): remove "Elm" and "Hana" after unibuild migration
	// completed.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		platformCondition := SkipOnPlatform(
			"bob", "elm", "fievel", "hana", "kevin", "kevin64", "oak", "tiger",
		)
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		modelCondition := SkipOnModel(
			"bob",
			"kevin",
			"kevin64",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	},
	}
}

// WifiNotMarvell8997 returns a hardware dependency condition that is satisfied if
// the DUT is not using Marvell 8997 chipsets.
func WifiNotMarvell8997() Condition {
	// TODO(b/187699768): replace this when we have hwdep for WiFi chips.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		platformCondition := SkipOnPlatform(
			"bob", "kevin", "kevin64",
		)
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		modelCondition := SkipOnModel(
			"bob",
			"kevin",
			"kevin64",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	},
	}
}

// WifiMarvell returns a hardware dependency condition that is satisfied if the
// the DUT is using a Marvell WiFi chip.
func WifiMarvell() Condition {
	// TODO(b/187699768): replace this when we have hwdep for WiFi chips.
	// TODO(b/187699664): remove "Elm" and "Hana" after unibuild migration
	// completed.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		platformCondition := Platform(
			"bob", "elm", "fievel", "hana", "kevin", "kevin64", "oak", "tiger",
		)
		if platformSatisfied, _, err := platformCondition.Satisfied(f); err == nil && platformSatisfied {
			return satisfied()
		}
		// bob, kevin may be the platform name or model name,
		// return satisfied if its plarform name or model name is bob/kevin
		modelCondition := Model(
			"bob", "kevin", "kevin64",
		)
		if modelSatisfied, _, err := modelCondition.Satisfied(f); err == nil && modelSatisfied {
			return satisfied()
		}
		return unsatisfied("DUT does not have a Marvell WiFi chip")
	},
	}
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
			"asurada", "bob", "cherry", "elm", "fievel", "gru", "grunt", "hana", "herobrine", "jacuzzi",
			"kevin", "kevin64", "kukui", "oak", "strongbad", "tiger", "trogdor", "trogdor-kernelnext",
		)
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		// NB: These exclusions are somewhat overly broad; for example, some
		// (but not all) blooglet devices have Intel WiFi chips. However,
		// for now there is no better way to specify the exact hardware
		// parameters needed for this dependency. (See crbug.com/1070299.)
		modelCondition := SkipOnModel(
			"beetley",
			"blipper",
			"blooglet",
			"dewatt",
			"dirinboz",
			"ezkinil",
			"gooey",
			"gumboz",
			"jelboz",
			"jelboz360",
			"lantis",
			"madoo",
			"nipperkin",
			"pirette",
			"pirika",
			"vilboz",
			"vorticon",
		)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	},
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
	},
	}
}

// WifiSAP returns a hardware dependency condition that if satisfied, indicates
// that a device supports SoftAP.
func WifiSAP() Condition {
	// TODO(crbug.com/1070299): we don't yet have relevant fields in device.Config
	// about WiFi chip, so list the known platforms here for now.
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		platformCondition := Platform(
			"strongbad", "trogdor", "trogdor-kernelnext",
		)
		if satisfied, reason, err := platformCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		return satisfied()
	},
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
	},
	}
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
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8195 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SC7180 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SC7280 {
			return unsatisfied("SoC does not support NV12 Overlays")
		}
		return satisfied()
	},
	}
}

// SupportsP010Overlays says true if the SoC supports P010 hardware overlays,
// which are commonly used for high bit-depth video overlays. Only Intel SoCs
// with GPU Gen 11 (JSL), Gen 12 (TGL, RLK) or later support those.
func SupportsP010Overlays() Condition {
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
			// Intel before Tiger Lake
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
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_U ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_Y ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_U_R ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_AMBERLAKE_Y ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_APOLLO_LAKE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_COMET_LAKE_U ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_GEMINI_LAKE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_WHISKEY_LAKE_U ||
			// All AMDs
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_STONEY_RIDGE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_PICASSO {
			return unsatisfied("SoC does not support P010 Overlays")
		}
		return satisfied()
	},
	}
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
	}}
}

// NvmeSelfTest returns a dependency condition if the device has an NVMe storage device which supported NVMe self-test.
func NvmeSelfTest() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.HasNvmeSelfTest {
			return satisfied()
		}
		return unsatisfied("DUT does not have an NVMe storage device which supports self-test")
	}}
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
	}}
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
	},
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
	},
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
	},
	}
}

var smartAmps = []string{
	configpb.HardwareFeatures_Audio_MAX98373.String(),
	configpb.HardwareFeatures_Audio_MAX98390.String(),
	configpb.HardwareFeatures_Audio_ALC1011.String(),
}

// SmartAmp returns a hardware dependency condition that is satisfied iff the DUT
// has smart amplifier.
func SmartAmp() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetAudio().GetSpeakerAmplifier() != nil {
			for _, amp := range smartAmps {
				if amp == hf.GetAudio().GetSpeakerAmplifier().GetName() {
					return satisfied()
				}
			}
		}
		return unsatisfied("DUT does not has smart amp :" + hf.GetAudio().GetSpeakerAmplifier().GetName())
	}}
}

// SmartAmpBootTimeCalibration returns a hardware dependency condition that is satisfied iff
// the DUT enables boot time calibration for smart amplifier.
func SmartAmpBootTimeCalibration() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetAudio().GetSpeakerAmplifier() != nil {
			for _, feature := range hf.GetAudio().GetSpeakerAmplifier().GetFeatures() {
				if feature == configpb.Component_Amplifier_BOOT_TIME_CALIBRATION {
					return satisfied()
				}
			}
		}
		return unsatisfied("DUT does not enable smart amp boot time calibration")
	}}
}

// formFactorListed returns whether the form factor represented by a configpb.HardwareFeatures
// is listed in the given list of form factor values.
func formFactorListed(hf *configpb.HardwareFeatures, ffList ...configpb.HardwareFeatures_FormFactor_FormFactorType) bool {
	for _, ffValue := range ffList {
		if hf.GetFormFactor().FormFactor == ffValue {
			return true
		}
	}
	return false
}

// FormFactor returns a hardware dependency condition that is satisfied
// iff the DUT's form factor is one of the given values.
func FormFactor(ffList ...configpb.HardwareFeatures_FormFactor_FormFactorType) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		listed := formFactorListed(hf, ffList...)
		if !listed {
			return unsatisfied("Form factor did not match")
		}
		return satisfied()
	}}
}

// SkipOnFormFactor returns a hardware dependency condition that is satisfied
// iff the DUT's form factor is none of the give values.
func SkipOnFormFactor(ffList ...configpb.HardwareFeatures_FormFactor_FormFactorType) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		listed := formFactorListed(hf, ffList...)
		if listed {
			return unsatisfied("Form factor matched to SkipOn list")
		}
		return satisfied()
	}}
}

// socTypeIsV4l2Stateful returns true when stateful API is supported on the given |SocType|
// or returns false when stateless API is supported.
func socTypeIsV4l2Stateful(SocType protocol.DeprecatedDeviceConfig_SOC) bool {
	switch SocType {
	case protocol.DeprecatedDeviceConfig_SOC_MT8173,
		protocol.DeprecatedDeviceConfig_SOC_SC7180,
		protocol.DeprecatedDeviceConfig_SOC_SC7280:
		return true
	case protocol.DeprecatedDeviceConfig_SOC_MT8183,
		protocol.DeprecatedDeviceConfig_SOC_MT8192,
		protocol.DeprecatedDeviceConfig_SOC_MT8195,
		protocol.DeprecatedDeviceConfig_SOC_MT8186,
		protocol.DeprecatedDeviceConfig_SOC_RK3399:
		return false
	// TODO(stevecho): stateful is more common for now, but we can change this in the future
	default:
		return true
	}
}

// SupportsV4L2StatefulVideoDecoding says true if the SoC supports the V4L2
// stateful video decoding kernel API. Examples of this are MTK8173 and
// Qualcomm devices (7180, etc). In general, we prefer to use stateless
// decoding APIs, so listing them individually makes sense.
func SupportsV4L2StatefulVideoDecoding() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86 || dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86_64 {
			return unsatisfied("DUT's CPU is x86 compatible, which doesn't support V4L2")
		}
		if socTypeIsV4l2Stateful(dc.GetSoc()) {
			return satisfied()
		}
		return unsatisfied("SoC does not support V4L2 Stateful HW video decoding")
	}}
}

// SupportsV4L2StatelessVideoDecoding says true if the SoC supports the V4L2
// stateless video decoding kernel API. Examples of this are MTK8192 (Asurada),
// MTK8195 (Cherry), MTK8186 (Corsola), and RK3399 (scarlet/kevin/bob).
func SupportsV4L2StatelessVideoDecoding() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86 || dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86_64 {
			return unsatisfied("DUT's CPU is x86 compatible, which doesn't support V4L2")
		}
		if !socTypeIsV4l2Stateful(dc.GetSoc()) {
			return satisfied()
		}
		return unsatisfied("SoC does not support V4L2 Stateless HW video decoding")
	}}
}

// Lid returns a hardware dependency condition that is satisfied iff the DUT's form factor has a lid.
func Lid() Condition {
	return FormFactor(Clamshell, Convertible, Detachable)
}

// InternalKeyboard returns a hardware dependency condition that is satisfied iff the DUT's form factor has a fixed undettachable keyboard.
func InternalKeyboard() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetKeyboard() == nil ||
			hf.GetKeyboard().KeyboardType != configpb.HardwareFeatures_Keyboard_INTERNAL {
			return unsatisfied("DUT does not have a fixed keyboard")
		}
		return satisfied()
	},
	}
}

// DisplayPortConverter is satisfied if a DP converter with one of the given names
// is present.
func DisplayPortConverter(names ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}

		for _, name := range names {
			for _, conv := range hf.GetDpConverter().GetConverters() {
				if conv.GetName() == name {
					return satisfied()
				}
			}
		}
		return unsatisfied("DP converter did not match")
	}}
}

// Vboot2 is satisfied iff crossystem param 'fw_vboot2' indicates that DUT uses vboot2.
func Vboot2() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.HasVboot2 {
			return satisfied()
		}
		return unsatisfied("DUT is not a vboot2 device")
	}}
}

// SupportsVP9KSVCHWDecoding is satisfied if the SoC supports VP9 k-SVC
// hardware decoding. They are x86 devices that are capable of VP9 hardware
// decoding and Qualcomm7180/7280.
// VP9 k-SVC is a SVC stream in which a frame only on keyframe can refer frames
// in a different spatial layer. See https://www.w3.org/TR/webrtc-svc/#dependencydiagrams* for detail.
func SupportsVP9KSVCHWDecoding() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}

		if dc.GetCpu() == protocol.DeprecatedDeviceConfig_X86_64 {
			return satisfied()
		}

		if dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SC7180 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SC7280 {
			return satisfied()
		}

		return unsatisfied("SoC does not support VP9 k-SVC HW decoding")
	}}
}

// SupportsVP9KSVCHWEncoding is satisfied if the SoC supports VP9 k-SVC
// hardware encoding. VP9 k-SVC is a SVC stream in which a frame only on keyframe
// can refer frames in a different spatial layer.
// See https://www.w3.org/TR/webrtc-svc/#dependencydiagrams* for detail.
func SupportsVP9KSVCHWEncoding() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_JASPER_LAKE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_TIGER_LAKE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_ALDER_LAKE {
			return satisfied()
		}
		return unsatisfied("SoC does not support VP9 k-SVC HW encoding")
	}}
}

// AssistantKey is satisfied if a model has an assistant key.
func AssistantKey() Condition {
	return Model("eve", "nocturne", "atlas")
}

// NoAssistantKey is satisfied if a model does not have an assistant key.
func NoAssistantKey() Condition {
	return SkipOnModel("eve", "nocturne", "atlas")
}
