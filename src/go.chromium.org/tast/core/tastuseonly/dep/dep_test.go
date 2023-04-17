// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/tastuseonly/dep"
	"go.chromium.org/tast/core/tastuseonly/protocol"
	"chromiumos/tast/testing/hwdep"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

func TestCheckDeps(t *testing.T) {
	d := &dep.Deps{Var: []string{"xyz"}}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"abc": "def"},
		},
	}
	if _, err := d.Check(f); err == nil {
		t.Error("Check with unsatisfied dependency unexpectedly succeeded")
	}

	// Disable dependency checks.
	f.CheckDeps = false

	if _, err := d.Check(f); err != nil {
		t.Errorf("Check with satisfied dependency failed: %v", err)
	}
}

func TestCheckSoftwareDepsSucceeded(t *testing.T) {
	d := &dep.Deps{
		Var: []string{"xyz"},
		Software: map[string]dep.SoftwareDeps{
			"":              []string{"sw1"},
			"CompanionDut1": []string{"sw2"},
		},
	}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"xyz": "def"},
		},
		Dut: &frameworkprotocol.DUTFeatures{
			Software: &frameworkprotocol.SoftwareFeatures{
				Available: []string{"sw1"},
			},
		},
		CompanionFeatures: map[string]*frameworkprotocol.DUTFeatures{
			"CompanionDut1": &frameworkprotocol.DUTFeatures{
				Software: &frameworkprotocol.SoftwareFeatures{
					Available: []string{"sw2"},
				},
			},
		},
	}

	if _, err := d.Check(f); err != nil {
		t.Errorf("Check with satisfied dependency failed: %v", err)
	}
}

func TestCheckSoftwareDepsPrimaryFailed(t *testing.T) {
	d := &dep.Deps{
		Var: []string{"xyz"},
		Software: map[string]dep.SoftwareDeps{
			"":              []string{"sw1"},
			"CompanionDut1": []string{"sw2"},
		},
	}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"xyz": "def"},
		},
		Dut: &frameworkprotocol.DUTFeatures{
			Software: &frameworkprotocol.SoftwareFeatures{
				Available: []string{"sw1err"},
			},
		},
		CompanionFeatures: map[string]*frameworkprotocol.DUTFeatures{
			"CompanionDut1": &frameworkprotocol.DUTFeatures{
				Software: &frameworkprotocol.SoftwareFeatures{
					Available: []string{"sw2"},
				},
			},
		},
	}

	if _, err := d.Check(f); err == nil {
		t.Error("Check with unsatisfied dependency unexpectedly succeeded")
	}
}

func TestCheckSoftwareDepsCompanionFailed(t *testing.T) {
	d := &dep.Deps{
		Var: []string{"xyz"},
		Software: map[string]dep.SoftwareDeps{
			"":              []string{"sw1"},
			"CompanionDut1": []string{"sw2"},
		},
	}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"xyz": "def"},
		},
		Dut: &frameworkprotocol.DUTFeatures{
			Software: &frameworkprotocol.SoftwareFeatures{
				Available: []string{"sw1"},
			},
		},
		CompanionFeatures: map[string]*frameworkprotocol.DUTFeatures{
			"CompanionDut1": &frameworkprotocol.DUTFeatures{
				Software: &frameworkprotocol.SoftwareFeatures{
					Available: []string{"errsw2"},
				},
			},
		},
	}

	if _, err := d.Check(f); err == nil {
		t.Error("Check with unsatisfied dependency unexpectedly succeeded")
	}
}

func TestCheckHardwareDepsSucceeded(t *testing.T) {
	d := &dep.Deps{
		Var: []string{"xyz"},
		Hardware: map[string]dep.HardwareDeps{
			"":              hwdep.D(hwdep.Model("samus")),
			"CompanionDut1": hwdep.D(hwdep.Model("samus2")),
		},
	}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"xyz": "def"},
		},
		Dut: &frameworkprotocol.DUTFeatures{
			Hardware: &frameworkprotocol.HardwareFeatures{
				DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
					Id: &frameworkprotocol.DeprecatedConfigId{

						Model: "samus",
					},
				},
			},
		},
		CompanionFeatures: map[string]*frameworkprotocol.DUTFeatures{
			"CompanionDut1": &frameworkprotocol.DUTFeatures{
				Hardware: &frameworkprotocol.HardwareFeatures{
					DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
						Id: &frameworkprotocol.DeprecatedConfigId{

							Model: "samus2",
						},
					},
				},
			},
		},
	}
	reasons, err := d.Check(f)
	if err != nil {
		t.Errorf("Check with satisfied dependency failed: %v", err)
	}

	if diff := cmp.Diff(reasons, []string(nil)); diff != "" {
		t.Errorf("Reasons unmatch (-got +want):\n%v", diff)
	}
}

func TestCheckHardwareDepsPrimaryFailed(t *testing.T) {
	d := &dep.Deps{
		Var: []string{"xyz"},
		Hardware: map[string]dep.HardwareDeps{
			"":              hwdep.D(hwdep.Model("samus")),
			"CompanionDut1": hwdep.D(hwdep.Model("samus2")),
		},
	}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"xyz": "def"},
		},
		Dut: &frameworkprotocol.DUTFeatures{
			Hardware: &frameworkprotocol.HardwareFeatures{
				DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
					Id: &frameworkprotocol.DeprecatedConfigId{

						Model: "samuserr",
					},
				},
			},
		},
		CompanionFeatures: map[string]*frameworkprotocol.DUTFeatures{
			"CompanionDut1": &frameworkprotocol.DUTFeatures{
				Hardware: &frameworkprotocol.HardwareFeatures{
					DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
						Id: &frameworkprotocol.DeprecatedConfigId{

							Model: "samus2",
						},
					},
				},
			},
		},
	}
	reasons, err := d.Check(f)
	if err != nil {
		t.Errorf("Check with satisfied dependency failed: %v", err)
	}

	if diff := cmp.Diff(reasons, []string{"ModelId did not match"}); diff != "" {
		t.Errorf("Reasons unmatch (-got +want):\n%v", diff)
	}
}

func TestCheckHardwareDepsCompanionFailed(t *testing.T) {
	d := &dep.Deps{
		Var: []string{"xyz"},
		Hardware: map[string]dep.HardwareDeps{
			"":              hwdep.D(hwdep.Model("samus")),
			"CompanionDut1": hwdep.D(hwdep.Model("samus2")),
		},
	}
	f := &protocol.Features{
		CheckDeps: true,
		Infra: &protocol.InfraFeatures{
			Vars: map[string]string{"xyz": "def"},
		},
		Dut: &frameworkprotocol.DUTFeatures{
			Hardware: &frameworkprotocol.HardwareFeatures{
				DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
					Id: &frameworkprotocol.DeprecatedConfigId{

						Model: "samus",
					},
				},
			},
		},
		CompanionFeatures: map[string]*frameworkprotocol.DUTFeatures{
			"CompanionDut1": &frameworkprotocol.DUTFeatures{
				Hardware: &frameworkprotocol.HardwareFeatures{
					DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
						Id: &frameworkprotocol.DeprecatedConfigId{

							Model: "samuserr",
						},
					},
				},
			},
		},
	}
	reasons, err := d.Check(f)
	if err != nil {
		t.Errorf("Check with satisfied dependency failed: %v", err)
	}

	if diff := cmp.Diff(reasons, []string{"ModelId did not match"}); diff != "" {
		t.Errorf("Reasons unmatch (-got +want):\n%v", diff)
	}
}
