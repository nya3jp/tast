// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"os"
	"os/exec"
	"path/filepath"

	"chromiumos/tast/common/testing"
	pow "chromiumos/tast/local/power"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CheckStatus,
		Desc: "Checks that dump_power_status can read power supply info from the kernel",
	})
}

func CheckStatus(s *testing.State) {
	status, err := pow.GetStatus()
	if err != nil {
		s.Fatal("Failed to get power status: ", err)
	}
	if status.BatteryPresent {
		if status.LinePowerConnected && status.BatteryDischarging {
			s.Error("Battery discharging while on line power")
		}
		if status.BatteryCharge > status.BatteryChargeFull {
			s.Errorf("Battery charge %0.1f exceeds full charge %0.1f",
				status.BatteryCharge, status.BatteryChargeFull)
		}
	} else {
		if !status.LinePowerConnected {
			s.Error("No battery found, but line power also not present. Call the Nobel Committee!")
		}
	}

	// For good measure, also write the output of power_supply_info to a file.
	b, err := exec.Command("power_supply_info").CombinedOutput()
	if err != nil {
		s.Error("power_supply_info failed: ", err)
	}
	if f, err := os.Create(filepath.Join(s.OutDir(), "power_supply_info.txt")); err != nil {
		s.Error(err)
	} else {
		defer f.Close()
		if _, err := f.Write(b); err != nil {
			s.Error(err)
		}
	}
}
