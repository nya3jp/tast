// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep

import (
	"testing"

	"go.chromium.org/chromiumos/infra/proto/go/device"
)

func TestSatisfied(t *testing.T) {
	d := D(Platform("Eve"), Model("eve"))
	if err := d.Satisfied(&device.Config{}); err == nil {
		t.Error("Unexpectedly satisfied the dependency")
	} else if len(err.Reasons) != 2 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}

	if err := d.Satisfied(&device.Config{
		Id: &device.ConfigId{
			PlatformId: &device.PlatformId{
				Value: "Eve",
			},
		},
	}); err == nil {
		t.Error("Unexpectedly satisfied the dependency")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}

	if err := d.Satisfied(&device.Config{
		Id: &device.ConfigId{
			ModelId: &device.ModelId{
				Value: "eve",
			},
		},
	}); err == nil {
		t.Error("Unexpectedly satisfied the dependency")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}

	if err := d.Satisfied(&device.Config{
		Id: &device.ConfigId{
			PlatformId: &device.PlatformId{
				Value: "Eve",
			},
			ModelId: &device.ModelId{
				Value: "eve",
			},
		},
	}); err != nil {
		t.Errorf("Unexpectedly unsatisified the dependency: %+v", err.Reasons)
	}
}

func TestMerge(t *testing.T) {
	d1 := D(Platform("Eve"))
	d2 := D(Model("eve"))
	d := Merge(d1, d2)

	if err := d.Satisfied(&device.Config{
		Id: &device.ConfigId{
			PlatformId: &device.PlatformId{
				Value: "Eve",
			},
		},
	}); err == nil {
		t.Error("Unexpectedly satisfied the dependency")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}

	if err := d.Satisfied(&device.Config{
		Id: &device.ConfigId{
			ModelId: &device.ModelId{
				Value: "eve",
			},
		},
	}); err == nil {
		t.Error("Unexpectedly satisfied the dependency")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}

	if err := d.Satisfied(&device.Config{
		Id: &device.ConfigId{
			PlatformId: &device.PlatformId{
				Value: "Eve",
			},
			ModelId: &device.ModelId{
				Value: "eve",
			},
		},
	}); err != nil {
		t.Errorf("Unexpectedly unsatisified the dependency: %+v", err.Reasons)
	}
}

func verifyCondition(t *testing.T, c Condition, dc *device.Config, expectSatisfied bool) {
	t.Helper()

	err := c(&deviceSetup{dc: dc})
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
			tc.expectSatisfied)
	}
}

func TestPlatform(t *testing.T) {
	c := Platform("Eve", "Kevin")

	for _, tc := range []struct {
		platform        string
		expectSatisfied bool
	}{
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
			tc.expectSatisfied)
	}
}

func TestSkipOnPlatform(t *testing.T) {
	c := SkipOnPlatform("Eve", "Kevin")

	for _, tc := range []struct {
		platform        string
		expectSatisfied bool
	}{
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
			tc.expectSatisfied)
	}
}
