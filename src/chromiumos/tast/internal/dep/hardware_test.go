// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"testing"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// success returns a HardwareCondition that is always satisfied.
func success() HardwareCondition {
	return HardwareCondition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		return true, "", nil
	}}
}

// fail returns a HardwareCondition that always fail to be satisfied.
func fail() HardwareCondition {
	return HardwareCondition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		return false, "failed", nil
	}}
}

// errCond returns a HardwareCondition that always returns error on evaluation.
func errCond() HardwareCondition {
	return HardwareCondition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		return false, "", errors.New("error in Satisfied()")
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
	if result, err := d.Satisfied(&protocol.HardwareFeatures{}); err != nil {
		t.Error("Unexpected error: ", err)
	} else if result != nil {
		t.Error("Unexpected fail: ", result)
	}
}

func TestHardwareDepsFail(t *testing.T) {
	d := NewHardwareDeps(fail())
	if err := d.Validate(); err != nil {
		t.Fatal("Unexpected validateion error: ", err)
	}
	if result, err := d.Satisfied(&protocol.HardwareFeatures{}); err != nil {
		t.Error("Unexpected error: ", err)
	} else if result == nil {
		t.Error("Unexpected success")
	}
}

func TestHardwareDepsInvalid(t *testing.T) {
	d := NewHardwareDeps(invalid())
	if err := d.Validate(); err == nil {
		t.Error("Unexpected validation pass")
	}
	// Make sure d.Satisfied() won't crash.
	if result, err := d.Satisfied(&protocol.HardwareFeatures{}); err != nil {
		t.Error("Unexpected error: ", err)
	} else if result == nil {
		t.Error("Unexpected success")
	}
}

func TestHardwareDepsMultipleCondition(t *testing.T) {
	d := NewHardwareDeps(success(), fail())
	if reasons, err := d.Satisfied(&protocol.HardwareFeatures{}); err != nil {
		t.Error("Unexpected error: ", err)
	} else if len(reasons) == 0 {
		t.Error("Unexpected success")
	} else if len(reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", reasons)
	}
}

func TestMergeHardwareDeps(t *testing.T) {
	d1 := NewHardwareDeps(success())
	d2 := NewHardwareDeps(fail())
	d := MergeHardwareDeps(d1, d2)

	if reasons, err := d.Satisfied(&protocol.HardwareFeatures{}); err != nil {
		t.Error("Unexpected error: ", err)
	} else if len(reasons) == 0 {
		t.Error("Unexpected success")
	} else if len(reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", reasons)
	}
}

func TestHardwareDepsError(t *testing.T) {
	d := NewHardwareDeps(errCond())
	if _, err := d.Satisfied(&protocol.HardwareFeatures{}); err == nil {
		t.Errorf("Unexpected success")
	}
}
