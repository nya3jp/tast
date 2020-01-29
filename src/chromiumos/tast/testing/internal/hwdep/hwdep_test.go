// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwdep

import (
	"testing"

	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
)

func success() Condition {
	return func(d *DeviceSetup) error {
		return nil
	}
}

func fail() Condition {
	return func(d *DeviceSetup) error {
		return errors.New("failed")
	}
}

func TestSatisfiedSuccess(t *testing.T) {
	d := D(success())
	if err := d.Satisfied(&device.Config{}); err != nil {
		t.Error("Unexpected fail: ", err)
	}
}

func TestSatisfiedFail(t *testing.T) {
	d := D(fail())
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
