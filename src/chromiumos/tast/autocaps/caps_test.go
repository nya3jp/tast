// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autocaps

import (
	"os"
	"reflect"
	"testing"

	"chromiumos/tast/testutil"
)

func TestCaps(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	const (
		managed  = "- cap_a\n- cap_b\n- cap_c\n- cap_d\n- cap_e\n- cap_f"
		base     = "- cap_a\n- cap_b\n- cap_c\n- cap_d"
		overlaid = "- no cap_b\n- disable cap_c"
		cpu      = `
- detector: intel_cpu
  match:
    - intel_celeron_2955U
  capabilities:
    - no cap_d
    - cap_e`
		cpuOther = `
- detector: intel_cpu
  match:
    - intel_i3_4005U
  capabilities:
    - no cap_e`
		kepler = `
- detector: kepler
  match:
    - kepler
  capabilities:
    - cap_f`
	)

	if err := testutil.WriteFiles(td, map[string]string{
		managedFile:         managed,
		"10-base.yaml":      base,
		"20-overlaid.yaml":  overlaid,
		"30-cpu.yaml":       cpu,
		"40-cpu-other.yaml": cpuOther,
		"50-kepler.yaml":    kepler,
	}); err != nil {
		t.Fatal(err)
	}

	info := SysInfo{
		CPUModel:  "Intel(R) Celeron(R) 2955U @ 1.40GHz",
		HasKepler: false,
	}
	caps, err := Read(td, &info)
	if err != nil {
		t.Fatalf("Read(%q, %+v) failed: %v", td, info, err)
	}

	exp := map[string]State{
		"cap_a": Yes,
		"cap_b": No,
		"cap_c": Disable,
		"cap_d": No,
		"cap_e": Yes,
		"cap_f": No,
	}
	if !reflect.DeepEqual(caps, exp) {
		t.Errorf("Read(%q, %+v) = %v; want %v", td, info, caps, exp)
	}

	// Now say that a Kepler device is present and check that the related capability is set.
	info.HasKepler = true
	if caps, err = Read(td, &info); err != nil {
		t.Fatalf("Read(%q, %+v) failed: %v", td, info, err)
	}
	exp["cap_f"] = Yes
	if !reflect.DeepEqual(caps, exp) {
		t.Errorf("Read(%q, %+v) = %v; want %v", td, info, caps, exp)
	}
}
