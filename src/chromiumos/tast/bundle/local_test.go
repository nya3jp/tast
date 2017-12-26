// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"os"
	gotesting "testing"

	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

func TestLocalBadArgs(t *gotesting.T) {
	args := []string{"-bogusflag"}
	if act, exp := Local(args), statusBadArgs; act != exp {
		t.Errorf("Local(%v) = %v; want %v", args, act, exp)
	}
}

func TestLocalBadTest(t *gotesting.T) {
	// A test without a function should trigger a registration error.
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{})

	args := []string{}
	if act, exp := Local(args), statusBadTests; act != exp {
		t.Errorf("Local(%v) = %v; want %v", args, act, exp)
	}
}

func TestLocalRunTest(t *gotesting.T) {
	const name = "pkg.Test"
	ran := false
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: name, Func: func(*testing.State) { ran = true }})

	outDir := testutil.TempDir(t, "bundle_test.")
	defer os.RemoveAll(outDir)
	args := []string{"-report", "-outdir=" + outDir, name}
	if act, exp := Local(args), statusSuccess; act != exp {
		t.Errorf("Local(%v) = %v; want %v", args, act, exp)
	}
	if !ran {
		t.Errorf("Local(%v) didn't run test %q", args, name)
	}
}
