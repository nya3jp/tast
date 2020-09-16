// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep

import (
	"testing"

	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/internal/dep"
)

func verifyCondition(t *testing.T, c Condition, dc *device.Config, features *configpb.HardwareFeatures, expectSatisfied bool) {
	t.Helper()

	err := c.Satisfied(&dep.HardwareFeatures{DC: dc, Features: features})
	if expectSatisfied {
		if err != nil {
			t.Error("Unexpectedly unsatisfied: ", err)
		}
	} else {
		if err == nil {
			t.Error("Unexpectedly satisfied")
		}
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
	} {
		verifyCondition(
			t, c,
			&device.Config{
				Id: &device.ConfigId{
					ModelId: &device.ModelId{
						Value: tc.model,
					},
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
	} {
		verifyCondition(
			t, c,
			&device.Config{
				Id: &device.ConfigId{
					ModelId: &device.ModelId{
						Value: tc.model,
					},
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
			&device.Config{
				Id: &device.ConfigId{
					PlatformId: &device.PlatformId{
						Value: tc.platform,
					},
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
			&device.Config{
				Id: &device.ConfigId{
					PlatformId: &device.PlatformId{
						Value: tc.platform,
					},
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
			&device.Config{
				Id: &device.ConfigId{
					PlatformId: &device.PlatformId{
						Value: "dummy_platform",
					},
				},
			},
			&configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					TouchSupport: tc.TouchSupport,
				},
			},
			tc.expectSatisfied)
	}

	verifyCondition(
		t, c,
		&device.Config{
			Id: &device.ConfigId{
				PlatformId: &device.PlatformId{
					Value: "dummy_platform",
				},
			},
			HardwareFeatures: []device.Config_HardwareFeature{
				device.Config_HARDWARE_FEATURE_TOUCHSCREEN,
			},
		},
		nil,
		true)
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
			&device.Config{
				Id: &device.ConfigId{
					PlatformId: &device.PlatformId{
						Value: "dummy_platform",
					},
				},
			},
			&configpb.HardwareFeatures{
				Fingerprint: &configpb.HardwareFeatures_Fingerprint{
					Location: tc.Fingerprint,
				},
			},
			tc.expectSatisfied)
	}

	verifyCondition(
		t, c,
		&device.Config{
			Id: &device.ConfigId{
				PlatformId: &device.PlatformId{
					Value: "dummy_platform",
				},
			},
			HardwareFeatures: []device.Config_HardwareFeature{
				device.Config_HARDWARE_FEATURE_FINGERPRINT,
			},
		},
		nil,
		true)
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
			&device.Config{
				Id: &device.ConfigId{
					PlatformId: &device.PlatformId{
						Value: "dummy_platform",
					},
				},
			},
			&configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					PanelProperties: tc.PanelProperties,
				},
			},
			tc.expectSatisfied)
	}

	verifyCondition(
		t, c,
		&device.Config{
			Id: &device.ConfigId{
				PlatformId: &device.PlatformId{
					Value: "dummy_platform",
				},
			},
			HardwareFeatures: []device.Config_HardwareFeature{
				device.Config_HARDWARE_FEATURE_INTERNAL_DISPLAY,
			},
		},
		nil,
		true)
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
		{D(InternalDisplay()), "dut.hardware_features.screen.panel_properties.diagonal_milliinch != 0"},
		{D(Wifi80211ac()), "dut.hardware_features.wifi.supported_wlan_protocols.exists(x, x == api.Component.Wifi.WLANProtocol.IEEE_802_11_AC)"},
		{D(WifiMACAddrRandomize()), "not_implemented"},
		{D(WifiNotMarvell()), "not_implemented"},

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
