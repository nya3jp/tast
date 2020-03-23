// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep

import (
	"testing"

	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
)

// success returns a Condition that is always satisified.
func success() Condition {
	return Condition{Satisfied: func(d *DeviceSetup) error {
		return nil
	}}
}

// fail returns a Condition that always fail to be satisfied.
func fail() Condition {
	return Condition{Satisfied: func(d *DeviceSetup) error {
		return errors.New("failed")
	}}
}

// invalid returns a Condition that always fail to be validated.
// This emulates, e.g., the situation that invalid argument is
// passed to a factory function to instantiate a Condition.
func invalid() Condition {
	return Condition{Err: errors.New("invalid condition")}
}

func TestSuccess(t *testing.T) {
	d := D(success())
	if err := d.Validate(); err != nil {
		t.Fatal("Unexpected validation error: ", err)
	}
	if err := d.Satisfied(&device.Config{}); err != nil {
		t.Error("Unexpected fail: ", err)
	}
}

func TestFail(t *testing.T) {
	d := D(fail())
	if err := d.Validate(); err != nil {
		t.Fatal("Unexpected validateion error: ", err)
	}
	if err := d.Satisfied(&device.Config{}); err == nil {
		t.Error("Unexpected success")
	}
}

func TestInvalid(t *testing.T) {
	d := D(invalid())
	if err := d.Validate(); err == nil {
		t.Error("Unexpected validation pass")
	}
	// Make sure d.Satisfied() won't crash.
	if err := d.Satisfied(&device.Config{}); err == nil {
		t.Error("Unexpected success")
	}
}

func TestMultipleCondition(t *testing.T) {
	d := D(success(), fail())
	if err := d.Satisfied(&device.Config{}); err == nil {
		t.Error("Unexpected success")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}
}

func TestMerge(t *testing.T) {
	d1 := D(success())
	d2 := D(fail())
	d := Merge(d1, d2)

	if err := d.Satisfied(&device.Config{}); err == nil {
		t.Error("Unexpected success")
	} else if len(err.Reasons) != 1 {
		t.Errorf("Unexpected number of reasons: got %+v", err.Reasons)
	}
}
