// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep

import (
	"testing"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/internal/protocol"
)

func verifyCondition(t *testing.T, c Condition, dc *protocol.DeprecatedDeviceConfig, features *configpb.HardwareFeatures, expectSatisfied bool) {
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

func expectError(t *testing.T, c Condition, dc *protocol.DeprecatedDeviceConfig, features *configpb.HardwareFeatures) {
	t.Helper()
	_, _, err := c.Satisfied(&protocol.HardwareFeatures{HardwareFeatures: features, DeprecatedDeviceConfig: dc})
	if err == nil {
		t.Errorf("Unexpectedly succeded")
	}
}

func TestModel(t *testing.T) {
	c := Model("eve", "kevin")

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
	c := SkipOnModel("eve", "kevin")

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
	c := Platform("eve", "kevin")

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
	c := SkipOnPlatform("eve", "kevin")

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
	c := TouchScreen()

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
	c := ChromeEC()

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

func TestFingerprint(t *testing.T) {
	c := Fingerprint()

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
	c := NoFingerprint()

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
	c := InternalDisplay()

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
	c := Nvme()

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

func TestCEL(t *testing.T) {
	for i, c := range []struct {
		input    Deps
		expected string
	}{
		{D(Model("model1", "model2")), "not_implemented"},
		{D(SkipOnModel("model1", "model2")), "not_implemented"},
		{D(Platform("platform_id1", "platform_id2")), "not_implemented"},
		{D(SkipOnPlatform("platform_id1", "platform_id2")), "not_implemented"},
		{D(TouchScreen()), "dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT"},
		{D(Fingerprint()), "dut.hardware_features.fingerprint.location != api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT"},
		{D(NoFingerprint()), "dut.hardware_features.fingerprint.location == api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT"},
		{D(InternalDisplay()), "dut.hardware_features.screen.panel_properties.diagonal_milliinch != 0"},
		{D(Wifi80211ac()), "dut.hardware_features.wifi.supported_wlan_protocols.exists(x, x == api.Component.Wifi.WLANProtocol.IEEE_802_11_AC)"},
		{D(Wifi80211ax()), "dut.hardware_features.wifi.supported_wlan_protocols.exists(x, x == api.Component.Wifi.WLANProtocol.IEEE_802_11_AX)"},
		{D(WifiMACAddrRandomize()), "not_implemented"},
		{D(WifiNotMarvell()), "not_implemented"},
		{D(Nvme()), "dut.hardware_features.storage.storage_type == api.Component.Storage.StorageType.NVME"},

		{D(TouchScreen(), Fingerprint()),
			"dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT && dut.hardware_features.fingerprint.location != api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT"},
		{D(Model("model1", "model2"), SkipOnPlatform("id1", "id2")), "not_implemented && not_implemented"},
	} {
		actual := c.input.CEL()
		if actual != c.expected {
			t.Errorf("TestCEL[%d]: got %q; want %q", i, actual, c.expected)
		}
	}
}

func TestWiFiIntel(t *testing.T) {
	c := WifiIntel()

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
	c := MinStorage(16)
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
	c := MinMemory(16000)
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
