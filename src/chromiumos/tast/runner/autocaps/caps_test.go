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
		managed  = "- cap_a\n- cap_b\n- cap_c\n- cap_d\n- cap_e"
		base     = "- cap_a\n- cap_b\n- cap_c\n- cap_d"
		overlaid = "- no cap_b\n- disable cap_c"
		detected = `
- detector: intel_cpu
  match:
    - intel_celeron_2955U
  capabilities:
    - no cap_d
    - cap_e`
	)

	if err := testutil.WriteFiles(td, map[string]string{
		managedFile:        managed,
		"10-base.yaml":     base,
		"20-overlaid.yaml": overlaid,
		"30-detected.yaml": detected,
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
	}
	if !reflect.DeepEqual(caps, exp) {
		t.Errorf("Read(%q, %+v) = %v; want %v", td, info, caps, exp)
	}
}
