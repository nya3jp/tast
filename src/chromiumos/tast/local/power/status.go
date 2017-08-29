// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package power interacts with power management on behalf of local tests.
package power

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Status holds power supply information reported by powerd's dump_power_status
// tool.
type Status struct {
	LinePowerConnected      bool
	BatteryPresent          bool
	BatteryDischarging      bool
	BatteryPercent          float64
	BatteryDisplayPercent   float64
	BatteryCharge           float64
	BatteryChargeFull       float64
	BatteryChargeFullDesign float64
	BatteryCurrent          float64
	BatteryEnergy           float64
	BatteryEnergyRate       float64
	BatteryVoltage          float64
}

// GetStatus returns current power supply information.
func GetStatus() (*Status, error) {
	b, err := exec.Command("dump_power_status").Output()
	if err != nil {
		return nil, err
	}

	m := make(map[string]string)

	getValue := func(k string) float64 {
		if err != nil {
			return 0.0
		}
		s := m[k]
		v := 0.0
		v, err = strconv.ParseFloat(s, 64)
		if err != nil {
			err = fmt.Errorf("key %q has non-float value %q", k, s)
			return 0.0
		}
		return v
	}

	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			break
		}
		f := strings.Fields(l)
		if len(f) != 2 {
			return nil, fmt.Errorf("didn't find two fields in line %q", l)
		}
		m[f[0]] = f[1]
	}

	s := &Status{
		LinePowerConnected:      getValue("line_power_connected") != 0.0,
		BatteryPresent:          getValue("battery_present") != 0.0,
		BatteryDischarging:      getValue("battery_discharging") != 0.0,
		BatteryPercent:          getValue("battery_percent"),
		BatteryDisplayPercent:   getValue("battery_display_percent"),
		BatteryCharge:           getValue("battery_charge"),
		BatteryChargeFull:       getValue("battery_charge_full"),
		BatteryChargeFullDesign: getValue("battery_charge_full_design"),
		BatteryCurrent:          getValue("battery_current"),
		BatteryEnergy:           getValue("battery_energy"),
		BatteryEnergyRate:       getValue("battery_energy_rate"),
		BatteryVoltage:          getValue("battery_voltage"),
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}
