// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crosconfig provides methods for interacting with the cros_config
// command line utility. See https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/chromeos-config
// for more information about cros_config.
package crosconfig

import (
	"context"
	"os/exec"
	"strings"

	"chromiumos/tast/caller"
)

var execCommandContext = exec.CommandContext

// HardwareProperty represents an attribute in /hardware-properties
// https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/chromeos-config#hardware_properties
type HardwareProperty string

const (
	// HasBaseAccelerometer does the dut have an accelerometer in its base
	HasBaseAccelerometer HardwareProperty = "has-base-accelerometer"
	// HasBaseGyroscope does the dut have a gyroscope in its base
	HasBaseGyroscope HardwareProperty = "has-base-gyroscope"
	// HasBaseMagnetometer does the dut have a gyroscope in its base
	HasBaseMagnetometer HardwareProperty = "has-base-magnetometer"
	// HasLidAccelerometer does the dut have an accelerometer in its lid
	HasLidAccelerometer HardwareProperty = "has-lid-accelerometer"
)

// allowedPkgs is the list of Go packages that can use this package.
// Currently only sensors is the only test suite allowed to use cros_config.
// cros_config should not be used as a general method of probing for features
// of a dut and adding new packages here will require approval.
var allowedPkgs = []string{
	"chromiumos/tast/local/bundles/cros/sensors",
	"chromiumos/tast/crosconfig",
	"main", // for local_test_runner
}

// CheckHardwareProperty returns true if the given hardware property is set to true and
// it returns false if the property is set to false or not set.
func CheckHardwareProperty(ctx context.Context, prop HardwareProperty) (bool, error) {
	caller.Check(2, allowedPkgs)

	cmd := execCommandContext(ctx, "cros_config", "/hardware-properties", string(prop))
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	if strings.TrimSpace(string(output)) == "true" {
		return true, nil
	}

	return false, nil
}
