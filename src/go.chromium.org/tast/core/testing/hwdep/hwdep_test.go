// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep_test

import (
	"testing"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

func verifyCondition(t *testing.T, c hwdep.Condition, dc *frameworkprotocol.DeprecatedDeviceConfig, features *configpb.HardwareFeatures, expectSatisfied bool) {
	t.Helper()

	satisfied, reason, err := c.Satisfied(&frameworkprotocol.HardwareFeatures{HardwareFeatures: features, DeprecatedDeviceConfig: dc})
	if err != nil {
		t.Error("Error while evaluating condition: ", err)
	}
	if expectSatisfied {
		if !satisfied {
			t.Error("Unexpectedly unsatisfied: ", reason)
		}
	} else {
		if satisfied {
			t.Error("Unexpectedly satisfied")
		}
	}
}

func expectError(t *testing.T, c hwdep.Condition, dc *frameworkprotocol.DeprecatedDeviceConfig, features *configpb.HardwareFeatures) {
	t.Helper()
	_, _, err := c.Satisfied(&frameworkprotocol.HardwareFeatures{HardwareFeatures: features, DeprecatedDeviceConfig: dc})
	if err == nil {
		t.Errorf("Unexpectedly succeded")
	}
}

func TestModel(t *testing.T) {
	c := hwdep.Model("eve", "kevin")

	for _, tc := range []struct {
		model           string
		expectSatisfied bool
	}{
		{"eve", true},
		{"kevin", true},
		{"nocturne", false},
		{"eve_signed", true},
		{"kevin_signed", true},
		{"nocturne_signed", false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Model: tc.model,
				},
			},
			&configpb.HardwareFeatures{},
			tc.expectSatisfied)
	}
}

func TestSkipOnModel(t *testing.T) {
	c := hwdep.SkipOnModel("eve", "kevin")

	for _, tc := range []struct {
		model           string
		expectSatisfied bool
	}{
		{"eve", false},
		{"kevin", false},
		{"nocturne", true},
		{"eve_signed", false},
		{"kevin_signed", false},
		{"nocturne_signed", true},
		{"", true}, // failed to get model Id
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Model: tc.model,
				},
			},
			&configpb.HardwareFeatures{},
			tc.expectSatisfied)
	}
}

func TestPlatform(t *testing.T) {
	c := hwdep.Platform("eve", "kevin")

	for _, tc := range []struct {
		platform        string
		expectSatisfied bool
	}{
		// Use capital letters to emulate actual cases.
		{"Eve", true},
		{"Kevin", true},
		{"Nocturne", false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Platform: tc.platform,
				},
			},
			&configpb.HardwareFeatures{},
			tc.expectSatisfied)
	}
}

func TestSkipOnPlatform(t *testing.T) {
	c := hwdep.SkipOnPlatform("eve", "kevin")

	for _, tc := range []struct {
		platform        string
		expectSatisfied bool
	}{
		// Use capital letters to emulate actual cases.
		{"Eve", false},
		{"Kevin", false},
		{"Nocturne", true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Platform: tc.platform,
				},
			},
			&configpb.HardwareFeatures{},
			tc.expectSatisfied)
	}
}

func TestTouchscreen(t *testing.T) {
	c := hwdep.TouchScreen()

	for _, tc := range []struct {
		TouchSupport    configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					TouchSupport: tc.TouchSupport,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestChromeEC(t *testing.T) {
	c := hwdep.ChromeEC()

	// Verify ECType defined with Boxster.
	for _, tc := range []struct {
		ECPresent       configpb.HardwareFeatures_Present
		ECType          configpb.HardwareFeatures_EmbeddedController_EmbeddedControllerType
		expectSatisfied bool
	}{
		{
			configpb.HardwareFeatures_PRESENT,
			configpb.HardwareFeatures_EmbeddedController_EC_CHROME,
			true,
		},
		{
			configpb.HardwareFeatures_PRESENT_UNKNOWN,
			configpb.HardwareFeatures_EmbeddedController_EC_CHROME,
			false,
		},
		{
			configpb.HardwareFeatures_NOT_PRESENT,
			configpb.HardwareFeatures_EmbeddedController_EC_CHROME,
			false,
		},
		{
			configpb.HardwareFeatures_PRESENT,
			configpb.HardwareFeatures_EmbeddedController_EC_TYPE_UNKNOWN,
			false,
		},
		{
			configpb.HardwareFeatures_PRESENT,
			configpb.HardwareFeatures_EmbeddedController_EC_WILCO,
			false,
		},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
					Present: tc.ECPresent,
					EcType:  tc.ECType,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestECFFeatureTypecCmd(t *testing.T) {
	c := hwdep.ECFeatureTypecCmd()

	// Verify ECType defined with Boxster.
	for _, tc := range []struct {
		ECFeatureTypecCmd configpb.HardwareFeatures_Present
		expectSatisfied   bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_PRESENT_UNKNOWN, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
					FeatureTypecCmd: tc.ECFeatureTypecCmd,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestFingerprint(t *testing.T) {
	c := hwdep.Fingerprint()

	for _, tc := range []struct {
		Fingerprint     bool
		expectSatisfied bool
	}{
		{true, true},
		{false, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Fingerprint: &configpb.HardwareFeatures_Fingerprint{
					Present: tc.Fingerprint,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestNoFingerprint(t *testing.T) {
	c := hwdep.NoFingerprint()

	for _, tc := range []struct {
		Fingerprint     bool
		expectSatisfied bool
	}{
		{true, false},
		{false, true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Fingerprint: &configpb.HardwareFeatures_Fingerprint{
					Present: tc.Fingerprint,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestFingerprintDiagSupported(t *testing.T) {
	c := hwdep.FingerprintDiagSupported()

	for _, tc := range []struct {
		DiagnosticRoutineEnabled bool
		expectSatisfied          bool
	}{
		{true, true},
		{false, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Fingerprint: &configpb.HardwareFeatures_Fingerprint{
					FingerprintDiag: &configpb.HardwareFeatures_Fingerprint_FingerprintDiag{
						RoutineEnable: tc.DiagnosticRoutineEnabled,
					},
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestInternalDisplay(t *testing.T) {
	c := hwdep.InternalDisplay()

	for _, tc := range []struct {
		PanelProperties *configpb.Component_DisplayPanel_Properties
		expectSatisfied bool
	}{
		{&configpb.Component_DisplayPanel_Properties{}, true},
		{nil, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					PanelProperties: tc.PanelProperties,
				},
			},
			tc.expectSatisfied)
	}
}

func TestEmmcStorage(t *testing.T) {
	c := hwdep.Emmc()

	for _, tc := range []struct {
		StorageType     configpb.Component_Storage_StorageType
		expectSatisfied bool
	}{
		{configpb.Component_Storage_STORAGE_TYPE_UNKNOWN, false},
		{configpb.Component_Storage_EMMC, true},
		{configpb.Component_Storage_NVME, false},
		{configpb.Component_Storage_SATA, false},
		{configpb.Component_Storage_UFS, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Storage: &configpb.HardwareFeatures_Storage{
					StorageType: tc.StorageType,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestNvmeStorage(t *testing.T) {
	c := hwdep.Nvme()

	for _, tc := range []struct {
		StorageType     configpb.Component_Storage_StorageType
		expectSatisfied bool
	}{
		{configpb.Component_Storage_NVME, true},
		{configpb.Component_Storage_STORAGE_TYPE_UNKNOWN, false},
		{configpb.Component_Storage_SATA, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Storage: &configpb.HardwareFeatures_Storage{
					StorageType: tc.StorageType,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestUfsStorage(t *testing.T) {
	c := hwdep.Ufs()

	for _, tc := range []struct {
		StorageType     configpb.Component_Storage_StorageType
		expectSatisfied bool
	}{
		{configpb.Component_Storage_STORAGE_TYPE_UNKNOWN, false},
		{configpb.Component_Storage_EMMC, false},
		{configpb.Component_Storage_NVME, false},
		{configpb.Component_Storage_SATA, false},
		{configpb.Component_Storage_UFS, true},
		{configpb.Component_Storage_BRIDGED_EMMC, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Storage: &configpb.HardwareFeatures_Storage{
					StorageType: tc.StorageType,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestWiFiIntel(t *testing.T) {
	c := hwdep.WifiIntel()

	for _, tc := range []struct {
		wifiDeviceID    wlan.DeviceID
		expectSatisfied bool
	}{
		{hwdep.Marvell88w8897SDIO, false},
		{hwdep.Marvell88w8997PCIE, false},
		{hwdep.QualcommAtherosQCA6174, false},
		{hwdep.QualcommAtherosQCA6174SDIO, false},
		{hwdep.QualcommWCN3990, false},
		{hwdep.QualcommWCN6750, false},
		{hwdep.QualcommWCN6855, false},
		{hwdep.Intel7260, true},
		{hwdep.Intel7265, true},
		{hwdep.Intel9000, true},
		{hwdep.Intel9260, true},
		{hwdep.Intel22260, true},
		{hwdep.Intel22560, true},
		{hwdep.IntelAX211, true},
		{hwdep.IntelBE200, true},
		{hwdep.BroadcomBCM4354SDIO, false},
		{hwdep.BroadcomBCM4356PCIE, false},
		{hwdep.BroadcomBCM4371PCIE, false},
		{hwdep.Realtek8822CPCIE, false},
		{hwdep.Realtek8852APCIE, false},
		{hwdep.Realtek8852CPCIE, false},
		{hwdep.MediaTekMT7921PCIE, false},
		{hwdep.MediaTekMT7921SDIO, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Wifi: &configpb.HardwareFeatures_Wifi{
					WifiChips: []configpb.HardwareFeatures_Wifi_WifiChip{configpb.HardwareFeatures_Wifi_WifiChip(tc.wifiDeviceID)},
				},
			},
			tc.expectSatisfied)
	}
}

func Test80211ax(t *testing.T) {
	c := hwdep.Wifi80211ax()

	for _, tc := range []struct {
		wifiDeviceID    wlan.DeviceID
		expectSatisfied bool
	}{
		{hwdep.Marvell88w8897SDIO, false},
		{hwdep.Marvell88w8997PCIE, false},
		{hwdep.QualcommAtherosQCA6174, false},
		{hwdep.QualcommAtherosQCA6174SDIO, false},
		{hwdep.QualcommWCN3990, false},
		{hwdep.QualcommWCN6750, true},
		{hwdep.QualcommWCN6855, true},
		{hwdep.Intel7260, false},
		{hwdep.Intel7265, false},
		{hwdep.Intel9000, false},
		{hwdep.Intel9260, false},
		{hwdep.Intel22260, true},
		{hwdep.Intel22560, true},
		{hwdep.IntelAX211, true},
		{hwdep.IntelBE200, true},
		{hwdep.BroadcomBCM4354SDIO, false},
		{hwdep.BroadcomBCM4356PCIE, false},
		{hwdep.BroadcomBCM4371PCIE, false},
		{hwdep.Realtek8822CPCIE, false},
		{hwdep.Realtek8852APCIE, true},
		{hwdep.Realtek8852CPCIE, true},
		{hwdep.MediaTekMT7921PCIE, true},
		{hwdep.MediaTekMT7921SDIO, true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Wifi: &configpb.HardwareFeatures_Wifi{
					WifiChips: []configpb.HardwareFeatures_Wifi_WifiChip{configpb.HardwareFeatures_Wifi_WifiChip(tc.wifiDeviceID)},
				},
			},
			tc.expectSatisfied)
	}
}

func TestWiFiNonSelfManaged(t *testing.T) {
	c := hwdep.WifiNonSelfManaged()

	for _, tc := range []struct {
		wifiDeviceID    wlan.DeviceID
		expectSatisfied bool
	}{
		{hwdep.Marvell88w8897SDIO, true},
		{hwdep.Marvell88w8997PCIE, true},
		{hwdep.QualcommAtherosQCA6174, true},
		{hwdep.QualcommAtherosQCA6174SDIO, true},
		{hwdep.QualcommWCN3990, true},
		{hwdep.QualcommWCN6750, false},
		{hwdep.QualcommWCN6855, false},
		{hwdep.Intel7260, false},
		{hwdep.Intel7265, false},
		{hwdep.Intel9000, false},
		{hwdep.Intel9260, false},
		{hwdep.Intel22260, false},
		{hwdep.Intel22560, false},
		{hwdep.IntelAX211, false},
		{hwdep.IntelBE200, false},
		{hwdep.BroadcomBCM4354SDIO, true},
		{hwdep.BroadcomBCM4356PCIE, true},
		{hwdep.BroadcomBCM4371PCIE, true},
		{hwdep.Realtek8822CPCIE, true},
		{hwdep.Realtek8852APCIE, true},
		{hwdep.Realtek8852CPCIE, true},
		{hwdep.MediaTekMT7921PCIE, true},
		{hwdep.MediaTekMT7921SDIO, true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Wifi: &configpb.HardwareFeatures_Wifi{
					WifiChips: []configpb.HardwareFeatures_Wifi_WifiChip{configpb.HardwareFeatures_Wifi_WifiChip(tc.wifiDeviceID)},
				},
			},
			tc.expectSatisfied)
	}
}

func TestMinStorage(t *testing.T) {
	c := hwdep.MinStorage(16)
	for _, tc := range []struct {
		sizeGb          uint32
		expectSatisfied bool
	}{
		{0, false},
		{15, false},
		{16, true},
		{32, true},
	} {
		verifyCondition(
			t, c,
			nil,
			&configpb.HardwareFeatures{
				Storage: &configpb.HardwareFeatures_Storage{
					SizeGb: tc.sizeGb,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		nil)
}

func TestMinMemory(t *testing.T) {
	c := hwdep.MinMemory(16000)
	for _, tc := range []struct {
		sizeMb          int32
		expectSatisfied bool
	}{
		{0, false},
		{15000, false},
		{16000, true},
		{32000, true},
	} {
		verifyCondition(
			t, c,
			nil,
			&configpb.HardwareFeatures{
				Memory: &configpb.HardwareFeatures_Memory{
					Profile: &configpb.Component_Memory_Profile{
						SizeMegabytes: tc.sizeMb,
					},
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		nil)
}

func TestMicrophone(t *testing.T) {
	c := hwdep.Microphone()

	for _, tc := range []struct {
		lidMicrophone   uint32
		baseMicrophone  uint32
		expectSatisfied bool
	}{
		{0, 0, false},
		{0, 1, true},
		{0, 2, true},
		{1, 0, true},
		{1, 1, true},
		{1, 2, true},
		{2, 0, true},
		{2, 1, true},
		{2, 2, true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Audio: &configpb.HardwareFeatures_Audio{
					LidMicrophone:  &configpb.HardwareFeatures_Count{Value: tc.lidMicrophone},
					BaseMicrophone: &configpb.HardwareFeatures_Count{Value: tc.baseMicrophone},
				},
			},
			tc.expectSatisfied)
	}
}

func TestSpeaker(t *testing.T) {
	c := hwdep.Speaker()

	for _, tc := range []struct {
		speakerAmplifier *configpb.Component_Amplifier
		expectSatisfied  bool
	}{
		{&configpb.Component_Amplifier{}, true},
		{nil, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Audio: &configpb.HardwareFeatures_Audio{
					SpeakerAmplifier: tc.speakerAmplifier,
				},
			},
			tc.expectSatisfied)
	}
}

func TestPrivacyScreen(t *testing.T) {
	c := hwdep.PrivacyScreen()

	for _, tc := range []struct {
		PrivacyScreen   configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				PrivacyScreen: &configpb.HardwareFeatures_PrivacyScreen{
					Present: tc.PrivacyScreen,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestKeyboard(t *testing.T) {
	c := hwdep.Keyboard()

	for _, tc := range []struct {
		features        *configpb.HardwareFeatures
		expectSatisfied bool
	}{
		{&configpb.HardwareFeatures{}, false},
		{&configpb.HardwareFeatures{
			Keyboard: &configpb.HardwareFeatures_Keyboard{},
		}, false},
		{&configpb.HardwareFeatures{
			Keyboard: &configpb.HardwareFeatures_Keyboard{
				KeyboardType: configpb.HardwareFeatures_Keyboard_DETACHABLE,
			},
		}, true},
		{&configpb.HardwareFeatures{
			Keyboard: &configpb.HardwareFeatures_Keyboard{
				KeyboardType: configpb.HardwareFeatures_Keyboard_INTERNAL,
			},
		}, true},
		{&configpb.HardwareFeatures{
			Keyboard: &configpb.HardwareFeatures_Keyboard{
				KeyboardType: configpb.HardwareFeatures_Keyboard_NONE,
			},
		}, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			tc.features,
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestSmartAmpBootTimeCalibration(t *testing.T) {
	c := hwdep.SmartAmpBootTimeCalibration()

	for _, tc := range []struct {
		features        *configpb.HardwareFeatures
		expectSatisfied bool
	}{
		{&configpb.HardwareFeatures{}, false},
		{&configpb.HardwareFeatures{
			Audio: &configpb.HardwareFeatures_Audio{
				SpeakerAmplifier: &configpb.Component_Amplifier{
					Features: []configpb.Component_Amplifier_Feature{configpb.Component_Amplifier_FEATURE_UNKNOWN},
				},
			},
		}, false},
		{&configpb.HardwareFeatures{
			Audio: &configpb.HardwareFeatures_Audio{
				SpeakerAmplifier: &configpb.Component_Amplifier{
					Features: []configpb.Component_Amplifier_Feature{configpb.Component_Amplifier_BOOT_TIME_CALIBRATION},
				},
			},
		}, true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			tc.features,
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestDisplayPortConverter(t *testing.T) {
	c := hwdep.DisplayPortConverter("PS175", "RTD2142")

	for _, tc := range []struct {
		features        *configpb.HardwareFeatures
		expectSatisfied bool
	}{
		{&configpb.HardwareFeatures{}, false},
		{&configpb.HardwareFeatures{
			DpConverter: &configpb.HardwareFeatures_DisplayPortConverter{
				Converters: []*configpb.Component_DisplayPortConverter{
					{Name: "RTD2141B"},
				},
			},
		}, false},
		{&configpb.HardwareFeatures{
			DpConverter: &configpb.HardwareFeatures_DisplayPortConverter{
				Converters: []*configpb.Component_DisplayPortConverter{
					{Name: "RTD2141B"},
					{Name: "PS175"},
				},
			},
		}, true},
	} {
		verifyCondition(t, c, &frameworkprotocol.DeprecatedDeviceConfig{},
			tc.features, tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestAssistantkey(t *testing.T) {
	for _, tc := range []struct {
		model         string
		wantSatisfied bool
	}{
		{"eve", true},
		{"nocturne", true},
		{"atlas", true},
		{"volteer", false},
	} {
		verifyCondition(t, hwdep.AssistantKey(), &frameworkprotocol.DeprecatedDeviceConfig{
			Id: &frameworkprotocol.DeprecatedConfigId{
				Model: tc.model,
			},
		}, &configpb.HardwareFeatures{}, tc.wantSatisfied)

		// hwdep.NoAssistantKey should always be !hwdep.AssistantKey.
		verifyCondition(t, hwdep.NoAssistantKey(), &frameworkprotocol.DeprecatedDeviceConfig{
			Id: &frameworkprotocol.DeprecatedConfigId{
				Model: tc.model,
			},
		}, &configpb.HardwareFeatures{}, !tc.wantSatisfied)
	}
}

func TestHapticTouchpad(t *testing.T) {
	for _, tc := range []struct {
		model         string
		wantSatisfied bool
	}{
		{"vell", true},
		{"redrix", true},
		{"volteer", false},
	} {
		verifyCondition(t, hwdep.HapticTouchpad(), &frameworkprotocol.DeprecatedDeviceConfig{
			Id: &frameworkprotocol.DeprecatedConfigId{
				Model: tc.model,
			},
		}, &configpb.HardwareFeatures{}, tc.wantSatisfied)
	}
}

func TestHPS(t *testing.T) {
	c := hwdep.HPS()

	for _, tc := range []struct {
		hpsPresent      configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Hps: &configpb.HardwareFeatures_Hps{
					Present: tc.hpsPresent,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestCameraFeature(t *testing.T) {
	for _, tc := range []struct {
		caps            []string
		features        []string
		expectSatisfied bool
	}{
		{[]string{"hdrnet", "gcam_ae"}, []string{}, true},
		{[]string{"hdrnet", "gcam_ae"}, []string{"hdrnet"}, true},
		{[]string{"hdrnet", "gcam_ae"}, []string{"hdrnet", "gcam_ae"}, true},
		{[]string{"hdrnet", "gcam_ae"}, []string{"auto_framing"}, false},
	} {
		verifyCondition(
			t, hwdep.CameraFeature(tc.features...),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Camera: &configpb.HardwareFeatures_Camera{
					Features: tc.caps,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, hwdep.CameraFeature(),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestHasTpm(t *testing.T) {
	for _, tc := range []struct {
		version         configpb.HardwareFeatures_TrustedPlatformModule_RuntimeTpmVersion
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_DISABLED, false},
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V1_2, true},
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V2, true},
	} {
		verifyCondition(
			t, hwdep.HasTpm(),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				TrustedPlatformModule: &configpb.HardwareFeatures_TrustedPlatformModule{
					RuntimeTpmVersion: tc.version,
				},
			},
			tc.expectSatisfied)
		verifyCondition(
			t, hwdep.HasNoTpm(),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				TrustedPlatformModule: &configpb.HardwareFeatures_TrustedPlatformModule{
					RuntimeTpmVersion: tc.version,
				},
			},
			!tc.expectSatisfied)
	}
	expectError(
		t, hwdep.HasTpm(),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
	expectError(
		t, hwdep.HasNoTpm(),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestHasTpm1(t *testing.T) {
	for _, tc := range []struct {
		version         configpb.HardwareFeatures_TrustedPlatformModule_RuntimeTpmVersion
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_DISABLED, false},
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V1_2, true},
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V2, false},
	} {
		verifyCondition(
			t, hwdep.HasTpm1(),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				TrustedPlatformModule: &configpb.HardwareFeatures_TrustedPlatformModule{
					RuntimeTpmVersion: tc.version,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, hwdep.HasTpm1(),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestHasTpm2(t *testing.T) {
	for _, tc := range []struct {
		version         configpb.HardwareFeatures_TrustedPlatformModule_RuntimeTpmVersion
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_DISABLED, false},
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V1_2, false},
		{configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V2, true},
	} {
		verifyCondition(
			t, hwdep.HasTpm2(),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				TrustedPlatformModule: &configpb.HardwareFeatures_TrustedPlatformModule{
					RuntimeTpmVersion: tc.version,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, hwdep.HasTpm2(),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestFirmwareKconfigFields(t *testing.T) {
	// hwdep.VBootCbfsIntegration()
	for _, tc := range []struct {
		present         configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
		{configpb.HardwareFeatures_PRESENT_UNKNOWN, false},
	} {
		verifyCondition(
			t, hwdep.VbootCbfsIntegration(),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				FwConfig: &configpb.HardwareFeatures_FirmwareConfiguration{
					VbootCbfsIntegration: tc.present,
				},
			},
			tc.expectSatisfied)
	}
	verifyCondition(
		t, hwdep.VbootCbfsIntegration(),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
		false)

	// hwdep.MainboardHasEarlyLibgfxinit()
	for _, tc := range []struct {
		present         configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
		{configpb.HardwareFeatures_PRESENT_UNKNOWN, false},
	} {
		verifyCondition(
			t, hwdep.MainboardHasEarlyLibgfxinit(),
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Model: "non-default",
				},
			},
			&configpb.HardwareFeatures{
				FwConfig: &configpb.HardwareFeatures_FirmwareConfiguration{
					MainboardHasEarlyLibgfxinit: tc.present,
				},
			},
			tc.expectSatisfied)
	}
	verifyCondition(
		t, hwdep.MainboardHasEarlyLibgfxinit(),
		&frameworkprotocol.DeprecatedDeviceConfig{
			Id: &frameworkprotocol.DeprecatedConfigId{
				Model: "non-default",
			},
		},
		nil,
		false)
	for _, model := range []string{"skolas", "brya0", "kano", "agah"} {
		verifyCondition(
			t, hwdep.MainboardHasEarlyLibgfxinit(),
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Model: model,
				},
			},
			nil,
			true)
		verifyCondition(
			t, hwdep.MainboardHasEarlyLibgfxinit(),
			&frameworkprotocol.DeprecatedDeviceConfig{
				Id: &frameworkprotocol.DeprecatedConfigId{
					Model: model,
				},
			},
			&configpb.HardwareFeatures{},
			true)
	}
}

func TestECBuildConfigOptions(t *testing.T) {
	testECBuildConfig := map[string]configpb.HardwareFeatures_Present{
		"CONFIG_A": configpb.HardwareFeatures_PRESENT,
		"CONFIG_B": configpb.HardwareFeatures_NOT_PRESENT,
		"CONFIG_C": configpb.HardwareFeatures_PRESENT_UNKNOWN,
	}
	for _, tc := range []struct {
		options   []string
		satisfied bool
	}{
		{[]string{"CONFIG_A"}, true},
		{[]string{"A"}, true},
		{[]string{"CONFIG_B"}, false},
		{[]string{"B"}, false},
		{[]string{"CONFIG_C"}, false},
		{[]string{"C"}, false},
		{[]string{"CONFIG_D"}, false},
		{[]string{"D"}, false},
		{[]string{""}, false},
		{[]string{"A", "B", "C", "D"}, true},
		{[]string{"D", "C", "B", "A"}, true},
		{[]string{"B", "C", "D"}, false},
		{[]string{"D", "C", "B"}, false},
	} {
		verifyCondition(
			t, hwdep.ECBuildConfigOptions(tc.options...),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
					BuildConfig: testECBuildConfig,
				},
			},
			tc.satisfied)
	}
	expectError(
		t, hwdep.ECBuildConfigOptions("FOO"),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestRuntimeProbeConfig(t *testing.T) {
	c := hwdep.RuntimeProbeConfig()

	for _, tc := range []struct {
		rpConfigPresent configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT, true},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				RuntimeProbeConfig: &configpb.HardwareFeatures_RuntimeProbeConfig{
					Present: tc.rpConfigPresent,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestRuntimeProbeConfigPrivate(t *testing.T) {
	for _, tc := range []struct {
		option                   bool
		rpEncryptedConfigPresent configpb.HardwareFeatures_Present
		expectSatisfied          bool
	}{
		{true, configpb.HardwareFeatures_PRESENT, true},
		{true, configpb.HardwareFeatures_NOT_PRESENT, false},
		{false, configpb.HardwareFeatures_PRESENT, false},
		{false, configpb.HardwareFeatures_NOT_PRESENT, true},
	} {
		verifyCondition(
			t, hwdep.RuntimeProbeConfigPrivate(tc.option),
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				RuntimeProbeConfig: &configpb.HardwareFeatures_RuntimeProbeConfig{
					EncryptedConfigPresent: tc.rpEncryptedConfigPresent,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, hwdep.RuntimeProbeConfigPrivate(true),
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil)
}

func TestGPUFamily(t *testing.T) {
	c := hwdep.GPUFamily("tigerlake", "qualcomm")
	for _, tc := range []struct {
		gpuFamily       string
		expectSatisfied bool
	}{
		{"tigerlake", true},
		{"qualcomm", true},
		{"rogue", false},
		{"rk3399", false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				HardwareProbeConfig: &configpb.HardwareFeatures_HardwareProbe{
					GpuFamily: tc.gpuFamily,
				},
			},
			tc.expectSatisfied,
		)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
	)
}

func TestSkipGPUFamily(t *testing.T) {
	c := hwdep.SkipGPUFamily("tigerlake", "qualcomm")
	for _, tc := range []struct {
		gpuFamily       string
		expectSatisfied bool
	}{
		{"tigerlake", false},
		{"qualcomm", false},
		{"rogue", true},
		{"rk3399", true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				HardwareProbeConfig: &configpb.HardwareFeatures_HardwareProbe{
					GpuFamily: tc.gpuFamily,
				},
			},
			tc.expectSatisfied,
		)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
	)
}

func TestGPUVendor(t *testing.T) {
	c := hwdep.GPUVendor("intel", "amd")
	for _, tc := range []struct {
		gpuVendor       string
		expectSatisfied bool
	}{
		{"intel", true},
		{"amd", true},
		{"vmware", false},
		{"nvidia", false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				HardwareProbeConfig: &configpb.HardwareFeatures_HardwareProbe{
					GpuVendor: tc.gpuVendor,
				},
			},
			tc.expectSatisfied,
		)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
	)
}

func TestSkipGPUVendor(t *testing.T) {
	c := hwdep.SkipGPUVendor("intel", "amd")
	for _, tc := range []struct {
		gpuVendor       string
		expectSatisfied bool
	}{
		{"intel", false},
		{"amd", false},
		{"vmware", true},
		{"nvidia", true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				HardwareProbeConfig: &configpb.HardwareFeatures_HardwareProbe{
					GpuVendor: tc.gpuVendor,
				},
			},
			tc.expectSatisfied,
		)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
	)
}

func TestCPUSocFamily(t *testing.T) {
	c := hwdep.CPUSocFamily("intel", "amd")
	for _, tc := range []struct {
		cpuSocFamily    string
		expectSatisfied bool
	}{
		{"intel", true},
		{"amd", true},
		{"qualcomm", false},
		{"mediatek", false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				HardwareProbeConfig: &configpb.HardwareFeatures_HardwareProbe{
					CpuSocFamily: tc.cpuSocFamily,
				},
			},
			tc.expectSatisfied,
		)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
	)
}

func TestSkipCPUSocFamily(t *testing.T) {
	c := hwdep.SkipCPUSocFamily("intel", "amd")
	for _, tc := range []struct {
		cpuSocFamily    string
		expectSatisfied bool
	}{
		{"intel", false},
		{"amd", false},
		{"qualcomm", true},
		{"mediatek", true},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				HardwareProbeConfig: &configpb.HardwareFeatures_HardwareProbe{
					CpuSocFamily: tc.cpuSocFamily,
				},
			},
			tc.expectSatisfied,
		)
	}
	expectError(
		t, c,
		&frameworkprotocol.DeprecatedDeviceConfig{},
		nil,
	)
}

func TestInternalTrackpoint(t *testing.T) {
	for _, tc := range []struct {
		model         string
		wantSatisfied bool
	}{
		{"morphius", true},
		{"eve", false},
		{"nocturne", false},
		{"volteer", false},
	} {
		verifyCondition(t, hwdep.InternalTrackpoint(), &frameworkprotocol.DeprecatedDeviceConfig{
			Id: &frameworkprotocol.DeprecatedConfigId{
				Model: tc.model,
			},
		}, &configpb.HardwareFeatures{}, tc.wantSatisfied)
	}
}

func TestFeatureLevel(t *testing.T) {
	c := hwdep.FeatureLevel(1)
	for _, tc := range []struct {
		level           uint32
		expectSatisfied bool
	}{
		{0, false},
		{1, true},
	} {
		verifyCondition(
			t, c,
			nil,
			&configpb.HardwareFeatures{
				FeatureLevel: tc.level,
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		nil)
}

func TestOEM(t *testing.T) {
	c := hwdep.OEM("Asus", "Dell")
	for _, tc := range []struct {
		oem             string
		expectSatisfied bool
	}{
		{"HP", false},
		{"Asus", true},
		{"Dell", true},
	} {
		t.Log("oem name: ", tc.oem)
		verifyCondition(
			t, c,
			nil,
			&configpb.HardwareFeatures{
				OemInfo: &configpb.HardwareFeatures_OEMInfo{
					Name: tc.oem,
				},
			},
			tc.expectSatisfied)
	}
}

func TestHasSideVolumeButton(t *testing.T) {
	c := hwdep.HasSideVolumeButton()

	for _, tc := range []struct {
		hasSideVolumeButton bool
		expectSatisfied     bool
	}{
		{true, true},
		{false, false},
	} {
		verifyCondition(
			t, c,
			&frameworkprotocol.DeprecatedDeviceConfig{
				HasSideVolumeButton: tc.hasSideVolumeButton,
			},
			&configpb.HardwareFeatures{},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		nil)
}

func TestHasBaseAccelerometer(t *testing.T) {
	c := hwdep.BaseAccelerometer()

	for _, tc := range []struct {
		baseAccelerometerPresent configpb.HardwareFeatures_Present
		expectSatisfied          bool
	}{
		{configpb.HardwareFeatures_PRESENT_UNKNOWN, false},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
		{configpb.HardwareFeatures_PRESENT, true},
	} {
		verifyCondition(
			t, c,
			nil,
			&configpb.HardwareFeatures{
				Accelerometer: &configpb.HardwareFeatures_Accelerometer{
					BaseAccelerometer: tc.baseAccelerometerPresent,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		nil)
}

func TestIntelIsh(t *testing.T) {
	c := hwdep.IntelIsh()

	for _, tc := range []struct {
		intelIshPresent configpb.HardwareFeatures_Present
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_PRESENT_UNKNOWN, false},
		{configpb.HardwareFeatures_NOT_PRESENT, false},
		{configpb.HardwareFeatures_PRESENT, true},
	} {
		verifyCondition(
			t, c,
			nil,
			&configpb.HardwareFeatures{
				FwConfig: &configpb.HardwareFeatures_FirmwareConfiguration{
					IntelIsh: tc.intelIshPresent,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		nil)

}
