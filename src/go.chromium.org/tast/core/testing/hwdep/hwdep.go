// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides the hardware dependency mechanism to select tests to run on
// a DUT based on its hardware features and setup.
package hwdep

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/dep"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/wlan"

	"go.chromium.org/tast/core/framework/protocol"
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

type FWUIType int

const (
	LegacyMenuUI FWUIType = iota
	LegacyClamshellUI
	MenuUI
)

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

// WLAN device IDs. Convenience wrappers.
const (
	Marvell88w8897SDIO         = wlan.Marvell88w8897SDIO
	Marvell88w8997PCIE         = wlan.Marvell88w8997PCIE
	QualcommAtherosQCA6174     = wlan.QualcommAtherosQCA6174
	QualcommAtherosQCA6174SDIO = wlan.QualcommAtherosQCA6174SDIO
	QualcommWCN3990            = wlan.QualcommWCN3990
	QualcommWCN6750            = wlan.QualcommWCN6750
	QualcommWCN6855            = wlan.QualcommWCN6855
	Intel7260                  = wlan.Intel7260
	Intel7265                  = wlan.Intel7265
	Intel8265                  = wlan.Intel8265
	Intel9000                  = wlan.Intel9000
	Intel9260                  = wlan.Intel9260
	Intel22260                 = wlan.Intel22260
	Intel22560                 = wlan.Intel22560
	IntelAX201                 = wlan.IntelAX201
	IntelAX203                 = wlan.IntelAX203
	IntelAX211                 = wlan.IntelAX211
	BroadcomBCM4354SDIO        = wlan.BroadcomBCM4354SDIO
	BroadcomBCM4356PCIE        = wlan.BroadcomBCM4356PCIE
	BroadcomBCM4371PCIE        = wlan.BroadcomBCM4371PCIE
	Realtek8822CPCIE           = wlan.Realtek8822CPCIE
	Realtek8852APCIE           = wlan.Realtek8852APCIE
	Realtek8852CPCIE           = wlan.Realtek8852CPCIE
	MediaTekMT7921PCIE         = wlan.MediaTekMT7921PCIE
	MediaTekMT7921SDIO         = wlan.MediaTekMT7921SDIO
	MediaTekMT7922PCIE         = wlan.MediaTekMT7922PCIE
)

// wifiDeviceListed returns whether a WiFi device given in HardwareFeatures is listed in the given list of names or not.
func wifiDeviceListed(hwf *protocol.HardwareFeatures, devices ...wlan.DeviceID) (bool, error) {
	wifi := hwf.GetHardwareFeatures().GetWifi()
	if wifi == nil {
		return false, errors.New("Wifi data has not been passed from DUT")
	}

	chipset := int32(wifi.WifiChips[0])

	for _, id := range devices {
		if id == wlan.DeviceID(chipset) {
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
// control, such as a device specific driver, or hardware etc., and unfortunately
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
		if err != nil {
			return withError(err)
		}
		if !listed {
			return unsatisfied("ModelId did not match")
		}
		return satisfied()
	}}
}

// SkipOnModel returns a hardware dependency condition that is satisfied
// if and only if the DUT's model ID is none of the given names.
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
// if and only if the DUT's platform ID is one of the give names.
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
// if and only if the DUT's platform ID is none of the give names.
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

// WifiDevice returns a hardware dependency condition that is satisfied
// if and only if the DUT's WiFi device is one of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func WifiDevice(devices ...wlan.DeviceID) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := wifiDeviceListed(f, devices...)
		if err != nil {
			// Fail-open. Assumption is that if the device is not recognized, it doesn't match.
			return unsatisfied(fmt.Sprintf("Unrecognized device. Assume not matching. Err %v", err))
		}
		if !listed {
			return unsatisfied("WiFi device did not match")
		}
		return satisfied()
	}}
}

// SkipOnWifiDevice returns a hardware dependency condition that is satisfied
// if and only if the DUT's WiFi device is none of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnWifiDevice(devices ...wlan.DeviceID) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := wifiDeviceListed(f, devices...)
		if err != nil {
			// Failed to get the device id.
			// Run the test to report error if it fails on this device.
			return satisfied()

		}
		if listed {
			return unsatisfied("WiFi device matched with skip-on list")
		}
		return satisfied()
	}}
}

// TouchScreen returns a hardware dependency condition that is satisfied
// if and only if the DUT has touchscreen.
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

// NoTouchScreen returns a hardware dependency condition that is satisfied
// if the DUT doesn't have a touchscreen.
func NoTouchScreen() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		if status := hf.GetScreen().GetTouchSupport(); status == configpb.HardwareFeatures_NOT_PRESENT {
			return satisfied()
		}
		return unsatisfied("DUT has a touchscreen")
	},
	}
}

// ChromeEC returns a hardware dependency condition that is satisfied
// if and only if the DUT has a present EC of the "Chrome EC" type.
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
// if and only if the DUT has an EC which supports the EC_FEATURE_TYPEC_CMD feature flag.
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
// is satisfied if and only if the DUT has an EC which supports CBI.
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

// ECFeatureDetachableBase returns a hardware dependency condition that is
// satisfied if and only if the DUT has the detachable base attached.
func ECFeatureDetachableBase() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		status := hf.GetEmbeddedController().GetDetachableBase()
		if status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("Detachable base is not attached to DUT")
		}
		if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine detachable base presence")
		}
		return satisfied()
	},
	}
}

// ECFeatureChargeControlV2 returns a hardware dependency condition that is
// satisfied if and only if the DUT supports version 2 of the EC_CMD_CHARGE_CONTROL feature
// (which adds battery sustain).
func ECFeatureChargeControlV2() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetEmbeddedController().GetFeatureChargeControlV2() == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT EC does not support EC_CMD_CHARGE_CONTROL version 2")
		}
		return satisfied()
	},
	}
}

// ECFeatureAssertsPanic returns a hardware dependency condition that is
// satisfied if and only if the DUT EC will panic on assertion failure.
func ECFeatureAssertsPanic() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		status := hf.GetEmbeddedController().GetFeatureAssertsPanic()
		if status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT EC does not panic on assert failure")
		}
		if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if DUT EC panics on assert failure")
		}
		return satisfied()
	},
	}
}

// ECFeatureSystemSafeMode returns a hardware dependency condition that is
// satisfied if and only if the DUT EC supports system safe mode recovery.
func ECFeatureSystemSafeMode() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		status := hf.GetEmbeddedController().GetFeatureSystemSafeMode()
		if status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT EC does not support system safe mode")
		}
		if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if DUT EC supports system safe mode")
		}
		return satisfied()
	},
	}
}

// ECFeatureMemoryDumpCommands returns a hardware dependency condition that is
// satisfied if and only if the DUT EC supports memory dump host commands.
func ECFeatureMemoryDumpCommands() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		status := hf.GetEmbeddedController().GetFeatureMemoryDumpCommands()
		if status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT EC does not support memory dump host commands")
		}
		if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if DUT EC supports memory dump host commands")
		}
		return satisfied()
	},
	}
}

// Cellular returns a hardware dependency condition that
// is satisfied if and only if the DUT has a cellular modem.
func Cellular() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have a cellular modem")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if cellular model is present")
		}
		return satisfied()
	},
	}
}

// SkipOnCellularVariant returns a hardware dependency condition that is satisfied
// if and only if the DUT's cellular variant is none of the given names.
func SkipOnCellularVariant(names ...string) Condition {
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("Variant should match with %v: %q", idRegexp, n)}
		}
	}
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have a cellular modem")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if cellular model is present")
		}
		variant := hf.GetCellular().GetModel()
		for _, name := range names {
			if name == variant {
				return unsatisfied("Variant matched with skip-on list")
			}
		}
		return satisfied()
	}}
}

// CellularVariant returns a hardware dependency condition that is satisfied
// if and only if the DUT's cellular variant is one of the given names.
func CellularVariant(names ...string) Condition {
	for _, n := range names {
		if !idRegexp.MatchString(n) {
			return Condition{Err: errors.Errorf("Variant should match with %v: %q", idRegexp, n)}
		}
	}
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have a cellular modem")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if cellular model is present")
		}
		variant := hf.GetCellular().GetModel()
		for _, name := range names {
			if name == variant {
				return satisfied()
			}
		}
		return unsatisfied("Variant did not match")
	}}
}

// CellularModemType returns a hardware dependency condition that is satisfied
// if and only if the DUT's cellular modem type is one of the given types.
func CellularModemType(modemTypes ...cellularconst.ModemType) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have a cellular modem")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if cellular model is present")
		}
		variant := hf.GetCellular().GetModel()
		modemType, err := cellularconst.GetModemTypeFromVariant(variant)
		if err != nil {
			return withError(err)
		}
		for _, m := range modemTypes {
			if m == modemType {
				return satisfied()
			}
		}
		return unsatisfied("Modem type did not match")
	}}
}

// SkipOnCellularModemType returns a hardware dependency condition that is satisfied
// if and only if the DUT's cellular modem type is none of the given types.
func SkipOnCellularModemType(modemTypes ...cellularconst.ModemType) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have a cellular modem")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if cellular model is present")
		}
		variant := hf.GetCellular().GetModel()
		modemType, err := cellularconst.GetModemTypeFromVariant(variant)
		if err != nil {
			return withError(err)
		}
		for _, m := range modemTypes {
			if m == modemType {
				return unsatisfied("Modem type matched with skip-on list")
			}
		}
		return satisfied()
	}}
}

// CellularSoftwareDynamicSar returns a hardware dependency condition that
// is satisfied if and only if the DUT has enabled software dynamic sar.
func CellularSoftwareDynamicSar() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().GetDynamicPowerReductionConfig().GetModemManager(); status {
			return satisfied()
		}
		return unsatisfied("DUT does not support cellular sw dynamic sar")
	},
	}
}

// NoCellular returns a hardware dependency condition that
// is satisfied if and only if the DUT does not have a cellular modem.
func NoCellular() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetCellular().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return satisfied()
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if cellular model is present")
		}
		return unsatisfied("DUT has a cellular modem")
	},
	}
}

// Bluetooth returns a hardware dependency condition that
// is satisfied if and only if the DUT has a bluetooth adapter.
func Bluetooth() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if hf := f.GetHardwareFeatures(); hf == nil {
			return withErrorStr("Did not find hardware features")
		} else if status := hf.GetBluetooth().Present; status == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("DUT does not have a bluetooth adapter")
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine bluetooth adapter presence")
		}
		return satisfied()
	},
	}
}

// GSCUART returns a hardware dependency condition that is satisfied if and only if the DUT has a GSC and that GSC has a working UART.
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

// GSCRWKeyIDProd returns a hardware dependency condition that
// is satisfied if and only if the DUT does have a GSC RW image signed with prod key.
func GSCRWKeyIDProd() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetTrustedPlatformModule().GetProductionRwKeyId(); status == configpb.HardwareFeatures_PRESENT {
			return satisfied()
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if production RW key is used to sign GSC image")
		}
		return unsatisfied("DUT has a dev signed GSC image")
	},
	}
}

// HasNoTpm returns a hardware dependency condition that is satisfied if and only if the DUT
// doesn't have an enabled TPM.
func HasNoTpm() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetRuntimeTpmVersion() != configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_DISABLED {
			return unsatisfied("DUT has an enabled TPM")
		}
		return satisfied()
	},
	}
}

// HasTpm returns a hardware dependency condition that is satisfied if and only if the DUT
// does have an enabled TPM.
func HasTpm() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetRuntimeTpmVersion() == configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_DISABLED {
			return unsatisfied("DUT has no enabled TPM")
		}
		return satisfied()
	},
	}
}

// HasTpm1 returns a hardware dependency condition that is satisfied if and only if the DUT
// does have an enabled TPM1.2.
func HasTpm1() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetRuntimeTpmVersion() == configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V1_2 {
			return satisfied()
		}
		return unsatisfied("DUT has no enabled TPM1.2")
	},
	}
}

// HasTpm2 returns a hardware dependency condition that is satisfied if and only if the DUT
// does have an enabled TPM2.0.
func HasTpm2() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetRuntimeTpmVersion() == configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V2 {
			return satisfied()
		}
		return unsatisfied("DUT has no enabled TPM2.0")
	},
	}
}

// HasGSCCr50 returns a hardware dependency condition that is satisfied if and only if the DUT
// does have a Cr50 GSC.
func HasGSCCr50() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetGscFwName() == configpb.HardwareFeatures_TrustedPlatformModule_GSC_CR50 {
			return satisfied()
		}
		return unsatisfied("DUT has no Cr50 GSC")
	},
	}
}

// HasGSCTi50 returns a hardware dependency condition that is satisfied if and only if the DUT
// does have a Ti50 GSC.
func HasGSCTi50() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetGscFwName() == configpb.HardwareFeatures_TrustedPlatformModule_GSC_TI50 {
			return satisfied()
		}
		return unsatisfied("DUT has no Ti50 GSC")
	},
	}
}

// HasTpmNvramRollbackSpace returns a hardware dependency condition that is satisfied if and only
// if the DUT has the TPM space to be used during enterprise enrollment.
func HasTpmNvramRollbackSpace() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetTrustedPlatformModule().GetEnterpriseRollbackSpace() == configpb.HardwareFeatures_PRESENT {
			return satisfied()
		}
		return unsatisfied("DUT's TPM has no enterprise rollback space (0x100e)")
	},
	}
}

// CPUNotNeedsCoreScheduling returns a hardware dependency condition that is satisfied if and only if the DUT's
// CPU is does not need to use core scheduling to mitigate hardware vulnerabilities.
func CPUNotNeedsCoreScheduling() Condition {
	return cpuNeedsCoreScheduling(false)
}

// CPUNeedsCoreScheduling returns a hardware dependency condition that is satisfied if and only if the DUT's
// CPU needs to use core scheduling to mitigate hardware vulnerabilities.
func CPUNeedsCoreScheduling() Condition {
	return cpuNeedsCoreScheduling(true)
}

// cpuNeedsCoreScheduling generates a Condition for CPUNeedsCoreScheduling() and its inverse,
// CPUNotNeedsCoreScheduling(). A CPU needs core scheduling if it is vulnerable to either L1TF or
// MDS hardware vulnerabilities.
func cpuNeedsCoreScheduling(enabled bool) Condition {
	needsCoreScheduling := func(hf *configpb.HardwareFeatures) (bool, string) {
		for _, f := range hf.GetSoc().Vulnerabilities {
			if f == configpb.Component_Soc_L1TF {
				return true, "CPU is vulnerable to L1TF"
			}
			if f == configpb.Component_Soc_MDS {
				return true, "CPU is vulnerable MDS"
			}
		}
		return false, "CPU is not vulnerable to L1TF or MDS"
	}
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		needed, description := needsCoreScheduling(hf)
		if needed == enabled {
			return satisfied()
		}
		return unsatisfied(description)
	},
	}
}

// CPUSupportsSMT returns a hardware dependency condition that is satisfied if and only if the DUT supports
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

// HasDynamicHighResTimerControl returns a hardware dependency condition that is satisfied if and only if the
// DUT has sysctl for turning off hrtimer.
func HasDynamicHighResTimerControl() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if _, err := os.Stat("/proc/sys/kernel/timer_highres"); err != nil {
			return unsatisfied("Device doesn't support dynamic hrtimer control")
		}
		if _, err := os.Stat("/proc/sys/kernel/sched_aggressive_next_balance"); err != nil {
			return unsatisfied("Device doesn't support dynamic hrtimer control")
		}
		if _, err := os.Stat("/proc/sys/kernel/sched_min_load_balance_interval"); err != nil {
			return unsatisfied("Device doesn't support dynamic hrtimer control")
		}
		return satisfied()
	},
	}
}

// CPUSupportsSHANI returns a hardware dependency condition that is satisfied if and only if the DUT supports
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

// Fingerprint returns a hardware dependency condition that is satisfied
// if and only if the DUT has fingerprint sensor.
func Fingerprint() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if !hf.GetFingerprint().GetPresent() {
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
		if hf.GetFingerprint().GetPresent() {
			return unsatisfied("DUT has fingerprint sensor")
		}
		return satisfied()
	},
	}
}

// FingerprintDiagSupported returns a hardware dependency condition that is
// satisfied if and only if the fingerprint diagnostic is supported on the DUT.
func FingerprintDiagSupported() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if !hf.GetFingerprint().GetFingerprintDiag().GetRoutineEnable() {
			return unsatisfied("DUT does not support fingerprint diagnostic routine")
		}
		return satisfied()
	},
	}
}

// VRR returns a hardware dependency condition that is satisfied if and only if
// the DUT is VRR Capable (has vrr_capable value set to 1 in modetest).
func VRR() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetVrr().GetPresent() != configpb.HardwareFeatures_PRESENT {
			return unsatisfied("DUT does not have VRR")
		}
		return satisfied()
	},
	}
}

// Display returns a hardware dependency condition that is satisfied if and
// only if the DUT has some display, internal or external.
func Display() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		display := hf.GetDisplay()
		if display == nil {
			return withErrorStr("Display is not given")
		}
		if display.GetType() == configpb.HardwareFeatures_Display_TYPE_UNKNOWN {
			return unsatisfied("DUT does not have a display")
		}
		return satisfied()
	},
	}
}

// ExternalDisplay returns a hardware dependency condition that is satisfied
// if and only if the DUT has an external display
func ExternalDisplay() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		display := hf.GetDisplay()
		if display == nil {
			return withErrorStr("Display is not given")
		}
		if display.GetType() == configpb.HardwareFeatures_Display_TYPE_EXTERNAL || display.GetType() == configpb.HardwareFeatures_Display_TYPE_INTERNAL_EXTERNAL {
			return satisfied()
		}
		return unsatisfied("DUT does not have an external display")
	},
	}
}

// HdmiConnected returns a hardware dependency condition that is satisfied
// if and only if the DUT has an external display with HDMI connected.
func HdmiConnected() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetHdmi().GetPresent() != configpb.HardwareFeatures_PRESENT {
			return unsatisfied("DUT does not have HDMI Connected")
		}
		return satisfied()
	},
	}
}

// InternalDisplay returns a hardware dependency condition that is satisfied
// if and only if the DUT has an internal display, e.g. Chromeboxes and Chromebits don't.
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
// if and only if the DUT does not have an internal display.
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
// if and only if the DUT has an keyboard, e.g. Chromeboxes and Chromebits don't.
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

// KeyboardBacklight returns a hardware dependency condition that is satisfied
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
// if and only if the DUT has a touchpad.
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
// if and only if the DUT's WiFi module supports 802.11ax.
func Wifi80211ax() Condition {
	return WifiDevice(
		QualcommWCN6750,
		QualcommWCN6855,
		Intel22260,
		Intel22560,
		IntelAX201,
		IntelAX203,
		IntelAX211,
		Realtek8852APCIE,
		Realtek8852CPCIE,
		MediaTekMT7921PCIE,
		MediaTekMT7921SDIO,
		MediaTekMT7922PCIE,
	)
}

// Wifi80211ax6E returns a hardware dependency condition that is satisfied
// if and only if the DUT's WiFi module supports WiFi 6E.
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
// if and only if the DUT supports WiFi MAC Address Randomization.
func WifiMACAddrRandomize() Condition {
	return SkipOnWifiDevice(
		// mwifiex in 3.10 kernel does not support it.
		Marvell88w8897SDIO, Marvell88w8997PCIE,
		// Broadcom driver has only NL80211_FEATURE_SCHED_SCAN_RANDOM_MAC_ADDR
		// but not NL80211_FEATURE_SCAN_RANDOM_MAC_ADDR. We require randomization
		// for all supported scan types.
		BroadcomBCM4354SDIO,
		// RTL8822CE reports only NL80211_FEATURE_SCAN_RANDOM_MAC_ADDR.
		Realtek8822CPCIE,
	)
}

// WifiTDLS returns a hardware dependency condition that is satisfied
// if and only if the DUT fully supports TDLS MGMT and OPER.
func WifiTDLS() Condition {
	return SkipOnWifiDevice(
		// QCA 6174 does not support TDLS.
		QualcommAtherosQCA6174, QualcommAtherosQCA6174SDIO,
		// MTK7921/SDIO (Pico6) has support issues.
		MediaTekMT7921SDIO,
	)
}

// WifiFT returns a hardware dependency condition that is satisfied
// if and only if the DUT supports Fast Transition roaming mode.
func WifiFT() Condition {
	return SkipOnWifiDevice(Marvell88w8897SDIO, Marvell88w8997PCIE)
}

// WifiNotMarvell returns a hardware dependency condition that is satisfied if and only if
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
		// return satisfied if its platform name or model name is bob/kevin
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
	return WifiDevice(
		Intel7260,
		Intel7265,
		Intel8265,
		Intel9000,
		Intel9260,
		Intel22260,
		Intel22560,
		IntelAX201,
		IntelAX203,
		IntelAX211,
	)
}

// WifiQualcomm returns a hardware dependency condition that if satisfied, indicates
// that a device uses Qualcomm WiFi.
func WifiQualcomm() Condition {
	return WifiDevice(
		QualcommAtherosQCA6174,
		QualcommAtherosQCA6174SDIO,
		QualcommWCN3990,
		QualcommWCN6750,
		QualcommWCN6855,
	)
}

// WifiNotQualcomm returns a hardware dependency condition that if satisfied, indicates
// that a device doesn't use Qualcomm WiFi.
func WifiNotQualcomm() Condition {
	return SkipOnWifiDevice(
		QualcommAtherosQCA6174,
		QualcommAtherosQCA6174SDIO,
		QualcommWCN3990,
		QualcommWCN6750,
		QualcommWCN6855,
	)
}

// WifiSAP returns a hardware dependency condition that if satisfied, indicates
// that a device supports SoftAP.
func WifiSAP() Condition {
	return WifiNotQualcomm()
}

// WifiP2P returns a hardware dependency condition that if satisfied, indicates
// that a device supports P2P.
func WifiP2P() Condition {
	return SkipOnWifiDevice(
		MediaTekMT7921PCIE,
		MediaTekMT7921SDIO,
		Realtek8822CPCIE,
		QualcommWCN6855,
	)
}

// These are the models which utilize SAR tables stored in VPD. See (b/204199379#comment10)
// for the methodology used to determine this list as well as a justification as
// to why it is stable.
var modelsWithVpdSarTables = []string{
	"akali360",
	"ampton",
	"arcada",
	"babytiger",
	"caroline",
	"eve",
	"leona",
	"nautilus",
	"nautiluslte",
	"pantheon",
	"shyvana",
	"vayne",
}

// WifiVpdSar returns a hardware dependency condition that if satisfied, indicates
// that a device supports VPD SAR tables, and the device actually has such tables
// in VPD.
func WifiVpdSar() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		modelCondition := Model(modelsWithVpdSarTables...)
		if satisfied, reason, err := modelCondition.Satisfied(f); err != nil || !satisfied {
			return satisfied, reason, err
		}
		wifi := f.GetHardwareFeatures().GetWifi()
		if wifi == nil {
			return unsatisfied("WiFi data has not been passed from DUT")
		}
		if !wifi.GetWifiVpdSar() {
			return unsatisfied("Device has no \"wifi_sar\" field in vpd")
		}
		return satisfied()
	},
	}
}

// WifiNoVpdSar returns a hardware dependency condition that if satisfied, indicates
// that the device does not support VPD SAR tables.
func WifiNoVpdSar() Condition {
	return SkipOnModel(modelsWithVpdSarTables...)
}

func hasBattery(f *protocol.HardwareFeatures) (bool, error) {
	dc := f.GetDeprecatedDeviceConfig()
	if dc == nil {
		return false, errors.New("DeprecatedDeviceConfig is not given")
	}
	return dc.GetPower() == protocol.DeprecatedDeviceConfig_POWER_SUPPLY_BATTERY, nil
}

// Battery returns a hardware dependency condition that is satisfied if and only if the DUT
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

// NoBatteryBootSupported returns a hardware dependency condition that is satisfied if and only if the DUT
// supports booting without a battery.
func NoBatteryBootSupported() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hasBattery, err := hasBattery(f)
		if err != nil {
			return withError(err)
		}
		if !hasBattery {
			return unsatisfied("DUT does not have a battery")
		}

		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if !hf.GetBattery().GetNoBatteryBootSupported() {
			return unsatisfied("DUT does not support booting without a battery")
		}

		return satisfied()
	},
	}
}

// SupportsHardwareOverlays returns a hardware dependency condition that is satisfied if the SoC
// supports hardware overlays.
func SupportsHardwareOverlays() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}

		if dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_STONEY_RIDGE ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_SC7180 {
			return unsatisfied("SoC does not support Hardware Overlays")
		}
		return satisfied()
	},
	}
}

// platformHasNV12Overlays returns true if the the given platform is known
// to support NV12 hardware overlays.
func platformHasNV12Overlays(SocType protocol.DeprecatedDeviceConfig_SOC) bool {
	return SocType != protocol.DeprecatedDeviceConfig_SOC_HASWELL &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_BAY_TRAIL &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_BROADWELL &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_BRASWELL &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_U &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_Y &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_APOLLO_LAKE &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_STONEY_RIDGE &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8173 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8176 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8183 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8192 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8195 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8186 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_MT8188G &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_SC7180 &&
		SocType != protocol.DeprecatedDeviceConfig_SOC_SC7280
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
		if !platformHasNV12Overlays(dc.GetSoc()) {
			return unsatisfied("SoC does not support NV12 Overlays")
		}
		return satisfied()
	},
	}
}

// SupportsVideoOverlays says true if the SoC supports some type of YUV
// hardware overlay. This includes NV12, I420, and YUY2.
func SupportsVideoOverlays() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}

		var supportsYUY2Overlays = dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8183 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8192 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8195 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8186 ||
			dc.GetSoc() == protocol.DeprecatedDeviceConfig_SOC_MT8188G
		if !platformHasNV12Overlays(dc.GetSoc()) && !supportsYUY2Overlays {
			return unsatisfied("SoC does not support Video Overlays")
		}
		return satisfied()
	},
	}
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

// ForceDischarge returns a hardware dependency condition that is satisfied if and only if the DUT
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

// Emmc returns a hardware dependency condition if the device has an eMMC
// storage device.
func Emmc() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetStorage().GetStorageType() == configpb.Component_Storage_EMMC {
			return satisfied()
		}
		return unsatisfied("DUT does not have an eMMC storage device")
	}}
}

// EmmcOverNvme returns a hardware dependency condition if the device has an eMMC
// storage device proxied by an eMMC to NVMe bridge (e.g. BH799)
func EmmcOverNvme() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetStorage().GetStorageType() == configpb.Component_Storage_BRIDGED_EMMC {
			return satisfied()
		}
		return unsatisfied("DUT does not have an eMMC over NVMe storage device")
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

// Ufs returns a hardware dependency condition if the device has a UFS storage
// device.
func Ufs() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetStorage().GetStorageType() == configpb.Component_Storage_UFS {
			return satisfied()
		}
		return unsatisfied("DUT does not have a UFS storage device")
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

// Speaker returns a hardware dependency condition that is satisfied if and only if the DUT has a speaker.
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

// Microphone returns a hardware dependency condition that is satisfied if and only if the DUT has a microphone.
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

// PrivacyScreen returns a hardware dependency condition that is satisfied if and only if the DUT has a privacy screen.
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

// NoPrivacyScreen returns a hardware dependency condition that is satisfied if the DUT
// does not have a privacy screen.
func NoPrivacyScreen() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if status := hf.GetPrivacyScreen().GetPresent(); status != configpb.HardwareFeatures_NOT_PRESENT {
			return satisfied()
		} else if status == configpb.HardwareFeatures_PRESENT_UNKNOWN {
			return unsatisfied("Could not determine if a privacy screen is present")
		}
		return unsatisfied("DUT has a privacy screen")
	},
	}
}

var smartAmps = []string{
	configpb.HardwareFeatures_Audio_MAX98373.String(),
	configpb.HardwareFeatures_Audio_MAX98390.String(),
	configpb.HardwareFeatures_Audio_ALC1011.String(),
	configpb.HardwareFeatures_Audio_CS35L41.String(),
}

// SmartAmp returns a hardware dependency condition that is satisfied if and only if the DUT
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

// SmartAmpBootTimeCalibration returns a hardware dependency condition that is satisfied if and only if
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
// if and only if the DUT's form factor is one of the given values.
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
// if and only if the DUT's form factor is none of the give values.
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
		protocol.DeprecatedDeviceConfig_SOC_MT8188G,
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

// SupportsV4L2FlatStatefulVideoDecoding says true if the SoC supports V4L2
// Flat stateful video decoding [1].
// [1] https://source.chromium.org/chromium/chromium/src/+/main:media/gpu/v4l2/v4l2_stateful_video_decoder.cc
func SupportsV4L2FlatStatefulVideoDecoding() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if satisfy, _, err := CPUSocFamily("qualcomm").Satisfied(f); err == nil && satisfy {
			return satisfied()
		}
		if satisfy, _, err := GPUFamily("rogue").Satisfied(f); err == nil && satisfy {
			return satisfied()
		}
		return unsatisfied("SoC does not support V4L2 Flat Stateful HW video decoding")
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

// SkipOnV4L2StatelessVideoDecoding says false if the SoC supports the V4L2
// stateless video decoding kernel API. Examples of this are MTK8192 (Asurada),
// MTK8195 (Cherry), MTK8186 (Corsola), and RK3399 (scarlet/kevin/bob).
func SkipOnV4L2StatelessVideoDecoding() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		dc := f.GetDeprecatedDeviceConfig()
		if dc == nil {
			return withErrorStr("DeprecatedDeviceConfig is not given")
		}
		if socTypeIsV4l2Stateful(dc.GetSoc()) {
			return satisfied()
		}
		return unsatisfied("SoC matches V4L2 Stateless HW video decoding SkipOn list")
	}}
}

// SupportsHEVCVideoDecodingInChrome says true if the device supports HEVC video
// decoding in Chrome. Note that this might be dif and only iferent from support at the
// platform (i.e. drivers, kernel, hardware) level.
// This function represents a policy: Tast tests using this should still make
// use of SoftwareDeps "caps.HWDecodeHEVC" (or "caps.HWDecodeHEVC10BPP").
func SupportsHEVCVideoDecodingInChrome() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		// Disabled on any device where the config is explicitly disabled.
		if f.GetHardwareFeatures().GetSoc().GetHevcSupport() == configpb.HardwareFeatures_NOT_PRESENT {
			return unsatisfied("Chrome has HEVC disabled on this device")
		}
		// Enabled on any device where the config is explicitly enabled.
		if f.GetHardwareFeatures().GetSoc().GetHevcSupport() == configpb.HardwareFeatures_PRESENT {
			return satisfied()
		}
		// ARM embedders don't support it.
		if satisfy, _, err := CPUSocFamily("mediatek", "rockchip", "qualcomm").Satisfied(f); err == nil && satisfy {
			return unsatisfied("Chrome does not support HEVC video decoding on this SoC")
		}
		// AMD Zork (Picasso) and before also don't.
		if satisfy, _, err := GPUFamily("picasso", "stoney").Satisfied(f); err == nil && satisfy {
			return unsatisfied("Chrome does not support HEVC video decoding on this SoC")
		}
		// Other AMD device support it.
		if satisfy, _, err := CPUSocFamily("amd").Satisfied(f); err == nil && satisfy {
			return satisfied()
		}
		// Any Intel TGL, ADL-P and MTL do support it.
		if satisfy, _, err := GPUFamily("tigerlake", "alderlake", "meteorlake").Satisfied(f); err == nil && satisfy {
			return satisfied()
		}
		return unsatisfied("Chrome does not support HEVC video decoding on this SoC")
	}}
}

// Lid returns a hardware dependency condition that is satisfied if and only if the DUT's form factor has a lid.
func Lid() Condition {
	return FormFactor(Clamshell, Convertible, Detachable)
}

// InternalKeyboard returns a hardware dependency condition that is satisfied if and only if the DUT's form factor has a fixed undetachable keyboard.
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

// NoInternalKeyboard returns a hardware dependency condition that is satisfied
// if and only if the DUT's form factor does not have a fixed undetachable
// keyboard.
func NoInternalKeyboard() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("HardwareFeatures is not given")
		}
		if hf.GetKeyboard() != nil &&
			hf.GetKeyboard().KeyboardType == configpb.HardwareFeatures_Keyboard_INTERNAL {
			return unsatisfied("DUT does have a fixed keyboard")
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

// Vboot2 is satisfied if and only if crossystem param 'fw_vboot2' indicates that DUT uses vboot2.
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
// in a dif and only iferent spatial layer. See https://www.w3.org/TR/webrtc-svc/#dependencydiagrams* for detail.
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

// AssistantKey is satisfied if a model has an assistant key.
func AssistantKey() Condition {
	return Model("eve", "nocturne", "atlas")
}

// NoAssistantKey is satisfied if a model does not have an assistant key.
func NoAssistantKey() Condition {
	return SkipOnModel("eve", "nocturne", "atlas")
}

// HapticTouchpad is satisfied if a model has a haptic touchpad.
func HapticTouchpad() Condition {
	return Model("vell", "redrix")
}

// HPS is satisfied if the HPS peripheral (go/cros-hps) is present in the DUT.
func HPS() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if hf := f.GetHardwareFeatures(); hf == nil {
			return withErrorStr("Did not find hardware features")
		} else if status := hf.GetHps().Present; status == configpb.HardwareFeatures_PRESENT {
			return satisfied()
		}
		return unsatisfied("HPS peripheral is not present")
	}}
}

func containsCameraFeature(strs []string, feature string) bool {
	for _, f := range strs {
		if f == feature {
			return true
		}
	}
	return false
}

// CameraFeature is satisfied if all the features listed in |names| are enabled on the DUT.
func CameraFeature(names ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if hf := f.GetHardwareFeatures(); hf == nil {
			return withErrorStr("Did not find hardware features")
		} else if features := hf.GetCamera().Features; features != nil {
			unsatisfiedFeatures := make([]string, 0, 10)
			for _, n := range names {
				if !containsCameraFeature(features, n) {
					unsatisfiedFeatures = append(unsatisfiedFeatures, n)
				}
			}
			if len(unsatisfiedFeatures) != 0 {
				return unsatisfied(fmt.Sprintf("Camera features not enabled: %v", unsatisfiedFeatures))
			}
			return satisfied()
		}
		return unsatisfied("Camera features not probed")
	}}
}

// CameraEnumerated is satisfied if all the camera devices are enumerated on the DUT.
func CameraEnumerated() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if hf := f.GetHardwareFeatures(); hf == nil {
			return withErrorStr("Did not find hardware features")
		} else if !hf.GetCamera().Enumerated {
			return unsatisfied("no camera was enumerated")
		}
		return satisfied()
	}}
}

func isAtLeastOneModuleListed(modules, enumerated []string) bool {
	for _, module := range modules {
		for _, id := range enumerated {
			if module == id {
				return true
			}
		}
	}
	return false
}

// CameraUSBModule is satisfied if at least one of the module is enumerated on the DUT.
func CameraUSBModule(modules ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if hf := f.GetHardwareFeatures(); hf == nil {
			return withErrorStr("Did not find hardware features")
		} else if enumerated := hf.GetCamera().EnumeratedUsbIds; enumerated != nil {
			if isAtLeastOneModuleListed(modules, enumerated) {
				return satisfied()
			}
			return unsatisfied("no USB Camera with given ID was enumerated")
		}
		return unsatisfied("no USB Camera was enumerated")
	}}
}

// SkipOnCameraUSBModule is satisfied if none of the given modules are enumerated.
// Note that the dependency is satisfied if no camera is enumerated. In some cases,
// this should be used with CameraEnumerated().
func SkipOnCameraUSBModule(modules ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		if hf := f.GetHardwareFeatures(); hf == nil {
			return withErrorStr("Did not find hardware features")
		} else if enumerated := hf.GetCamera().EnumeratedUsbIds; enumerated != nil {
			if isAtLeastOneModuleListed(modules, enumerated) {
				return unsatisfied("matched with skip-on list")
			}
			return satisfied()
		}
		return satisfied()
	}}
}

// MainboardHasEarlyLibgfxinit is satisfied if the BIOS was built with Kconfig CONFIG_MAINBOARD_HAS_EARLY_LIBGFXINIT
func MainboardHasEarlyLibgfxinit() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf != nil {
			fwc := hf.GetFwConfig()
			if fwc != nil {
				if fwc.MainboardHasEarlyLibgfxinit == configpb.HardwareFeatures_PRESENT {
					return satisfied()
				}
				if fwc.MainboardHasEarlyLibgfxinit == configpb.HardwareFeatures_NOT_PRESENT {
					return unsatisfied("MainboardHasEarlyLibgfxinit Kconfig disabled")
				}
			}
		}
		// Some Brya models default to PRESENT
		listed, err := modelListed(f.GetDeprecatedDeviceConfig(), "skolas", "brya0", "kano", "agah", "taeko", "crota", "osiris", "gaelen", "lisbon", "gladios", "marasov", "omnigul", "constitution")
		if err != nil {
			return withError(err)
		}
		if listed {
			return satisfied()
		}
		// The default for this Kconfig is off, so not found is the same as disabled.
		return unsatisfied("Kconfig not found")
	}}
}

// VbootCbfsIntegration is satisfied if the BIOS was built with Kconfig CONFIG_VBOOT_CBFS_INTEGRATION
func VbootCbfsIntegration() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf != nil {
			fwc := hf.GetFwConfig()
			if fwc != nil {
				if fwc.VbootCbfsIntegration == configpb.HardwareFeatures_PRESENT {
					return satisfied()
				}
				if fwc.VbootCbfsIntegration == configpb.HardwareFeatures_NOT_PRESENT {
					return unsatisfied("VbootCbfsIntegration Kconfig disabled")
				}
			}
		}
		// The default for this Kconfig is off, so not found is the same as disabled.
		return unsatisfied("Kconfig not found")
	}}
}

// RuntimeProbeConfig is satisfied if the probe config of the model exists.
func RuntimeProbeConfig() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		if hf.GetRuntimeProbeConfig().GetPresent() != configpb.HardwareFeatures_PRESENT {
			return unsatisfied("DUT does not have Runtime Probe config")
		}
		return satisfied()
	}}
}

// SeamlessRefreshRate is satisfied if the device supports changing refresh rates without modeset.
func SeamlessRefreshRate() Condition {
	// TODO: Determine at runtime if a device meets the requirements by inspecting EDID, kernel, and SoC versions.
	return Model("mithrax", "taniks")
}

// GPUFamily is satisfied if the devices GPU family is categorized as one of the families specified.
// For a complete list of values or to add new ones please check the pciid maps at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func GPUFamily(families ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		for _, family := range families {
			if hf.GetHardwareProbeConfig().GetGpuFamily() == family {
				return satisfied()
			}
		}
		return unsatisfied("DUT GPU family is not met")
	}}
}

// SkipGPUFamily is satisfied if the devices GPU family is none of the families specified.
// For a complete list of values or to add new ones please check the pciid maps at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func SkipGPUFamily(families ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		for _, family := range families {
			if hf.GetHardwareProbeConfig().GetGpuFamily() == family {
				return unsatisfied("DUT GPU family matched with skip list")
			}
		}
		return satisfied()
	}}
}

// GPUVendor is satisfied if the devices GPU vendor is categorized as one of the vendors specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func GPUVendor(vendors ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		for _, vendor := range vendors {
			if hf.GetHardwareProbeConfig().GetGpuVendor() == vendor {
				return satisfied()
			}
		}
		return unsatisfied("DUT GPU vendor is not met")
	}}
}

// SkipGPUVendor is satisfied if the devices GPU vendor is categorized as none of the vendors specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func SkipGPUVendor(vendors ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		for _, vendor := range vendors {
			if hf.GetHardwareProbeConfig().GetGpuVendor() == vendor {
				return unsatisfied("DUT GPU vendor matched with skip list")
			}
		}
		return satisfied()
	}}
}

// CPUSocFamily is satisfied if the devices CPU SOC family is categorized as one of the families specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func CPUSocFamily(families ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		for _, family := range families {
			if hf.GetHardwareProbeConfig().GetCpuSocFamily() == family {
				return satisfied()
			}
		}
		return unsatisfied("DUT CPU soc family is not met")
	}}
}

// SkipCPUSocFamily is satisfied if the device's CPU SOC family is none of the families specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func SkipCPUSocFamily(families ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("DUT HardwareFeatures data is not given")
		}
		for _, family := range families {
			if hf.GetHardwareProbeConfig().GetCpuSocFamily() == family {
				return unsatisfied("DUT CPU soc family matched with skip list")
			}
		}
		return satisfied()
	}}
}

// InternalTrackpoint is satisfied if a model has an internal trackpoint.
func InternalTrackpoint() Condition {
	return Model("morphius")
}

// FeatureLevel is satisfied if the feature level of the DUT match the value of
// the parameter level.
func FeatureLevel(level uint32) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf == nil {
			return withErrorStr("Did not find hardware features")
		}
		if hf.GetFeatureLevel() != level {
			return unsatisfied(fmt.Sprintf("The DUT has different feature level; got %d, need %d",
				hf.GetFeatureLevel(), level))
		}
		return satisfied()
	}}
}

// OEM is satisfied if the OEM names on the DUT is in the input allow list.
func OEM(names ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		oem := f.GetHardwareFeatures().GetOemInfo().GetName()
		for _, name := range names {
			if name == oem {
				return satisfied()
			}
		}
		return unsatisfied("DUT OEM name [" + oem + "] is not in the allow list")
	}}
}

// SkipOnOEM is satisfied if the OEM names on the DUT is not in the input allow list.
func SkipOnOEM(names ...string) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		oem := f.GetHardwareFeatures().GetOemInfo().GetName()
		for _, name := range names {
			if name == oem {
				return unsatisfied("DUT OEM name [" + oem + "] is in the allow list")
			}
		}
		return satisfied()
	}}
}

// legacyMenuUIModels contains models adopting LegacyMenuUI.
var legacyMenuUIModels = []string{
	"krane",
	"kodama",
	"katsu",
	"kakadu",
	"nocturne",
	"soraka",
	"dru",
	"druwl",
	"dumo",
}

// fwUIListed returns whether the dut's firmware ui is listed in the given list
// of firmware ui types. It uses the machine's active firmware version
// represented by a protocol.HardwareFeatures to determine whether menu UI is
// implemented. If not, it will check the legacyMenuUIModels list for
// differentiation between LegacyMenuUI and LegacyClamshellUI.
func fwUIListed(f *protocol.HardwareFeatures, names ...FWUIType) (bool, error) {
	fwid := f.GetHardwareFeatures().GetFwConfig().FwRwActiveVersion
	if fwid == 0 {
		return false, errors.New("firmware id has not been passed from the DUT")
	}
	var dutFWUI FWUIType
	// Using chromium: 2043102 as a reference, which landed in R83-12992.0.0,
	// firmware versions greater than '12992' should have the menu UI
	// implemented.
	if fwid > 12992 {
		dutFWUI = MenuUI
	} else {
		isLegacyMenuUI, err := modelListed(f.GetDeprecatedDeviceConfig(), legacyMenuUIModels...)
		if err != nil {
			return false, err
		}
		if isLegacyMenuUI {
			dutFWUI = LegacyMenuUI
		} else {
			dutFWUI = LegacyClamshellUI
		}
	}
	for _, name := range names {
		if name == dutFWUI {
			return true, nil
		}
	}
	return false, nil
}

// FirmwareUIType returns a dependency condition that is satisfied if
// the DUT's firmware UI is one of the given types.
func FirmwareUIType(fwUIType ...FWUIType) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		listed, err := fwUIListed(f, fwUIType...)
		if err != nil {
			return withError(err)
		}
		if !listed {
			return unsatisfied("FWUIType did not match")
		}
		return satisfied()
	}}
}

// HasPDPort returns a hardware dependency condition that is satisfied
// if and only if the DUT has a USB-C PD port with the provided index.
// I.e. HasPDPort(1) will match on devices that have 2 or more USB-C PD ports.
func HasPDPort(port uint32) Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		hf := f.GetHardwareFeatures()
		if hf.GetUsbC().GetCount() == nil {
			return unsatisfied("Did not find USB-C port count")
		}
		if hf.GetUsbC().GetCount().GetValue() > port {
			return satisfied()
		}
		return unsatisfied(fmt.Sprintf("DUT does not have PD port %d", port))
	},
	}
}

// AlternativeFirmware returns a hardware dependency condition that is satisfied if and only if the DUT has altfw.
func AlternativeFirmware() Condition {
	return Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		sc := f.GetSoftwareConfig()
		if sc == nil {
			return withErrorStr("Did not find software config")
		}
		if sc.GetAltFirmwareConfig() == nil {
			return withErrorStr("Did not find alt firmware config")
		}
		if sc.GetAltFirmwareConfig().HasAltFirmware {
			return satisfied()
		}
		return unsatisfied("DUT does not have altfw")
	},
	}
}
