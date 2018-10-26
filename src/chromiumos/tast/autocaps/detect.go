// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autocaps

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var cpuModelRegexp *regexp.Regexp // matches "Model name" line from lscpu

func init() {
	cpuModelRegexp = regexp.MustCompile(`^Model name:\s+(.+)`)
}

const (
	// PCI vendor:device ID string for Kepler.
	keplerID = "1ae0:001a"
)

// SysInfo contains information about the system.
type SysInfo struct {
	// CPUModel contains the value from the "Model name:" line printed by lscpu.
	CPUModel string
	// HasKepler is true if "lspci -n -d <keplerID>" prints non-empty output.
	HasKepler bool
}

// loadSysInfo returns a SysInfo struct describing the system where this code is running.
func loadSysInfo() (*SysInfo, error) {
	info := SysInfo{}
	out, err := exec.Command("lscpu").Output()
	if err != nil {
		return nil, fmt.Errorf("lscpu failed: %v", err)
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if ms := cpuModelRegexp.FindStringSubmatch(ln); ms != nil {
			info.CPUModel = strings.TrimSpace(ms[1])
			break
		}
	}

	// The lspci command can fail if the device doesn't have a PCI bus: https://crbug.com/888883
	if out, err = exec.Command("lspci", "-n", "-d", keplerID).Output(); err == nil {
		info.HasKepler = strings.TrimSpace(string(out)) != ""
	}

	return &info, nil
}

// cpuModelMap maps from SysInfo.CPUModel values to the corresponding detector match values.
// This comes from client/cros/video/detectors/intel_cpu.py in the Autotest repo.
var cpuModelMap = map[string]string{
	"Intel(R) Celeron(R) 2955U @ 1.40GHz":      "intel_celeron_2955U",
	"Intel(R) Celeron(R) 2957U @ 1.40GHz":      "intel_celeron_2957U",
	"Intel(R) Celeron(R) CPU 1007U @ 1.50GHz":  "intel_celeron_1007U",
	"Intel(R) Celeron(R) CPU 847 @ 1.10GHz":    "intel_celeron_847",
	"Intel(R) Celeron(R) CPU 867 @ 1.30GHz":    "intel_celeron_867",
	"Intel(R) Celeron(R) CPU 877 @ 1.40GHz":    "intel_celeron_877",
	"Intel(R) Celeron(R) CPU B840 @ 1.90GHz":   "intel_celeron_B840",
	"Intel(R) Core(TM) i3-4005U CPU @ 1.70GHz": "intel_i3_4005U",
	"Intel(R) Core(TM) i3-4010U CPU @ 1.70GHz": "intel_i3_4010U",
	"Intel(R) Core(TM) i3-4030U CPU @ 1.90GHz": "intel_i3_4030U",
	"Intel(R) Core(TM) i5-2450M CPU @ 2.50GHz": "intel_i5_2450M",
	"Intel(R) Core(TM) i5-2467M CPU @ 1.60GHz": "intel_i5_2467M",
	"Intel(R) Core(TM) i5-2520M CPU @ 2.50GHz": "intel_i5_2520M",
	"Intel(R) Core(TM) i5-3427U CPU @ 1.80GHz": "intel_i7_3427U",
	"Intel(R) Core(TM) i7-4600U CPU @ 2.10GHz": "intel_i7_4600U",
}

// runDetector returns the directives contained in rule.Capabilities if rule matches info.
func runDetector(rule *detectRule, info *SysInfo) (directives []string, err error) {
	var val string
	switch rule.Detector {
	case "intel_cpu":
		val = cpuModelMap[info.CPUModel]
	case "kepler":
		if info.HasKepler {
			val = "kepler"
		}
	default:
		return nil, fmt.Errorf("unknown detector %q", rule.Detector)
	}

	for _, m := range rule.Match {
		if m == val {
			return rule.Capabilities, nil
		}
	}
	return nil, nil
}
