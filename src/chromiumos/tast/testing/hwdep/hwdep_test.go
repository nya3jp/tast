// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep_test

import (
	"testing"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/testing/hwdep"
)

func verifyCondition(t *testing.T, c hwdep.Condition, dc *protocol.DeprecatedDeviceConfig, features *configpb.HardwareFeatures, expectSatisfied bool) {
	t.Helper()

	satisfied, reason, err := c.Satisfied(&protocol.HardwareFeatures{HardwareFeatures: features, DeprecatedDeviceConfig: dc})
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

func expectError(t *testing.T, c hwdep.Condition, dc *protocol.DeprecatedDeviceConfig, features *configpb.HardwareFeatures) {
	t.Helper()
	_, _, err := c.Satisfied(&protocol.HardwareFeatures{HardwareFeatures: features, DeprecatedDeviceConfig: dc})
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
			&protocol.DeprecatedDeviceConfig{
				Id: &protocol.DeprecatedConfigId{
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
			&protocol.DeprecatedDeviceConfig{
				Id: &protocol.DeprecatedConfigId{
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
			&protocol.DeprecatedDeviceConfig{
				Id: &protocol.DeprecatedConfigId{
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
			&protocol.DeprecatedDeviceConfig{
				Id: &protocol.DeprecatedConfigId{
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
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					TouchSupport: tc.TouchSupport,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
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
		&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
					FeatureTypecCmd: tc.ECFeatureTypecCmd,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
		nil)
}

func TestFingerprint(t *testing.T) {
	c := hwdep.Fingerprint()

	for _, tc := range []struct {
		Fingerprint     configpb.HardwareFeatures_Fingerprint_Location
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_Fingerprint_NOT_PRESENT, false},
		{configpb.HardwareFeatures_Fingerprint_LOCATION_UNKNOWN, true},
	} {
		verifyCondition(
			t, c,
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Fingerprint: &configpb.HardwareFeatures_Fingerprint{
					Location: tc.Fingerprint,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
		nil)
}

func TestNoFingerprint(t *testing.T) {
	c := hwdep.NoFingerprint()

	for _, tc := range []struct {
		Fingerprint     configpb.HardwareFeatures_Fingerprint_Location
		expectSatisfied bool
	}{
		{configpb.HardwareFeatures_Fingerprint_NOT_PRESENT, true},
		{configpb.HardwareFeatures_Fingerprint_LOCATION_UNKNOWN, false},
	} {
		verifyCondition(
			t, c,
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Fingerprint: &configpb.HardwareFeatures_Fingerprint{
					Location: tc.Fingerprint,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					PanelProperties: tc.PanelProperties,
				},
			},
			tc.expectSatisfied)
	}
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
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				Storage: &configpb.HardwareFeatures_Storage{
					StorageType: tc.StorageType,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
		nil)
}

func TestWiFiIntel(t *testing.T) {
	c := hwdep.WifiIntel()

	for _, tc := range []struct {
		platform        string
		model           string
		expectSatisfied bool
	}{
		{"grunt", "barla", false},
		{"zork", "ezkinil", false},
		{"zork", "morphius", true},
		{"octopus", "droid", true},
	} {
		verifyCondition(
			t, c,
			&protocol.DeprecatedDeviceConfig{
				Id: &protocol.DeprecatedConfigId{
					Platform: tc.platform,
					Model:    tc.model,
				},
			},
			&configpb.HardwareFeatures{},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		nil,
		&configpb.HardwareFeatures{})
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
			&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
			&configpb.HardwareFeatures{
				PrivacyScreen: &configpb.HardwareFeatures_PrivacyScreen{
					Present: tc.PrivacyScreen,
				},
			},
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
			tc.features,
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
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
			&protocol.DeprecatedDeviceConfig{},
			tc.features,
			tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
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
					&configpb.Component_DisplayPortConverter{Name: "RTD2141B"},
				},
			},
		}, false},
		{&configpb.HardwareFeatures{
			DpConverter: &configpb.HardwareFeatures_DisplayPortConverter{
				Converters: []*configpb.Component_DisplayPortConverter{
					&configpb.Component_DisplayPortConverter{Name: "RTD2141B"},
					&configpb.Component_DisplayPortConverter{Name: "PS175"},
				},
			},
		}, true},
	} {
		verifyCondition(t, c, &protocol.DeprecatedDeviceConfig{},
			tc.features, tc.expectSatisfied)
	}
	expectError(
		t, c,
		&protocol.DeprecatedDeviceConfig{},
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
		verifyCondition(t, hwdep.AssistantKey(), &protocol.DeprecatedDeviceConfig{
			Id: &protocol.DeprecatedConfigId{
				Model: tc.model,
			},
		}, &configpb.HardwareFeatures{}, tc.wantSatisfied)

		// hwdep.NoAssistantKey should always be !hwdep.AssistantKey.
		verifyCondition(t, hwdep.NoAssistantKey(), &protocol.DeprecatedDeviceConfig{
			Id: &protocol.DeprecatedConfigId{
				Model: tc.model,
			},
		}, &configpb.HardwareFeatures{}, !tc.wantSatisfied)
	}
}
