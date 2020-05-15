// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"testing"

	"chromiumos/tast/errors"
)

// success returns a HardwareCondition that is always satisfied.
func success() HardwareCondition {
	return HardwareCondition{Satisfied: func(f *HardwareFeatures) error {
		return nil
	}}
}

// fail returns a HardwareCondition that always fail to be satisfied.
func fail() HardwareCondition {
	return HardwareCondition{Satisfied: func(f *HardwareFeatures) error {
		return errors.New("failed")
	}}
}

// invalid returns a HardwareCondition that always fail to be validated.
// This emulates, e.g., the situation that invalid argument is
// passed to a factory function to instantiate a HardwareCondition.
func invalid() HardwareCondition {
	return HardwareCondition{Err: errors.New("invalid condition")}
}

func TestHardwareDepsSuccess(t *testing.T) {
	d := NewHardwareDeps(success())
	if err := d.Validate(); err != nil {
		t.Fatal("Unexpected validation error: ", err)
	}
	if err := d.Satisfied(&HardwareFeatures{}); err != nil {
		t.Error("Unexpected fail: ", err)
	}
}

func TestHardwareDepsFail(t *testing.T) {
	d := NewHardwareDeps(fail())
	if err := d.Validate(); err != nil {
		t.Fatal("Unexpected validateion error: ", err)
	}
	if err := d.Satisfied(&HardwareFeatures{}); err == nil {
		t.Error("Unexpected success")
	}
}

func TestHardwareDepsInvalid(t *testing.T) {
	d := NewHardwareDeps(invalid())
	if err := d.Validate(); err == nil {
		t.Error("Unexpected validation pass")
	}
	// Make sure d.Satisfied() won't crash.
	if err := d.Satisfied(&HardwareFeatures{}); err == nil {
		t.Error("Unexpected success")
	}
}

func TestHardwareDepsMultipleCondition(t *testing.T) {
	d := NewHardwareDeps(success(), fail())
	if err := d.Satisfied(&HardwareFeatures{}); err == nil {
		t.Error("Unexpected success")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}
}

func TestMergeHardwareDeps(t *testing.T) {
	d1 := NewHardwareDeps(success())
	d2 := NewHardwareDeps(fail())
	d := MergeHardwareDeps(d1, d2)

	if err := d.Satisfied(&HardwareFeatures{}); err == nil {
		t.Error("Unexpected success")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}
}
