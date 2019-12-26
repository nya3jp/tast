// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"os"
	"strings"
	gotesting "testing"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

func TestLocalBadTest(t *gotesting.T) {
	// A test without a function should trigger a registration error.
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTest(&testing.Test{})

	args := Args{Mode: RunTestsMode}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, LocalDelegate{}); status != statusBadTests {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusBadTests)
	}
	if len(stderr.String()) == 0 {
		t.Errorf("Local(%+v) didn't write error to stderr", args)
	}
}

func TestLocalRunTest(t *gotesting.T) {
	const name = "pkg.Test"
	ran := false
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: name, Func: func(context.Context, *testing.State) { ran = true }})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := Args{Mode: RunTestsMode, RunTests: &RunTestsArgs{OutDir: outDir}}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, LocalDelegate{}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if !ran {
		t.Errorf("Local(%+v) didn't run test %q", args, name)
	}
	if len(stderr.String()) != 0 {
		t.Errorf("Local(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}
}

func TestLocalReadyFunc(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	// Ensure that a successful ready function is executed.
	args := Args{
		Mode: RunTestsMode,
		RunTests: &RunTestsArgs{
			OutDir:         outDir,
			WaitUntilReady: true,
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	ranReady := false
	ready := func(context.Context, func(string)) error {
		ranReady = true
		return nil
	}
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, LocalDelegate{
		Ready: ready,
	}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if !ranReady {
		t.Errorf("Local(%+v) didn't run ready function", args)
	}

	// Local should fail if the ready function returns an error.
	stdin = newBufferWithArgs(t, &args)
	stderr = bytes.Buffer{}
	const msg = "intentional failure"
	ready = func(context.Context, func(string)) error { return errors.New(msg) }
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, LocalDelegate{
		Ready: ready,
	}); status != statusError {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusError)
	}
	if s := stderr.String(); !strings.Contains(s, msg) {
		t.Errorf("Local(%+v) didn't write ready error %q to stderr (got %q)", args, msg, s)
	}
}

func TestLocalReadyFuncDisabled(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	// The ready function should be skipped if WaitUntilReady is false.
	args := Args{
		Mode: RunTestsMode,
		RunTests: &RunTestsArgs{
			OutDir:         outDir,
			WaitUntilReady: false,
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	ranReady := false
	ready := func(context.Context, func(string)) error {
		ranReady = true
		return nil
	}
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, LocalDelegate{
		Ready: ready,
	}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if ranReady {
		t.Errorf("Local(%+v) ran ready function despite being told not to", args)
	}
}

func TestLocalPreTestRun(t *gotesting.T) {
	const name = "pkg.Test"
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: name, Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := Args{Mode: RunTestsMode, RunTests: &RunTestsArgs{OutDir: outDir}}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	var ranPre, ranPostHook bool
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, LocalDelegate{
		PreTestRun: func(context.Context, *testing.State) func(context.Context, *testing.State) {
			ranPre = true
			return func(context.Context, *testing.State) {
				ranPostHook = true
			}
		},
	}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if !ranPre {
		t.Errorf("Local(%+v) didn't run test pre %q", args, name)
	}
	if !ranPostHook {
		t.Errorf("Local(%+v) didn't run test post hook %q", args, name)
	}
	if len(stderr.String()) != 0 {
		t.Errorf("Local(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}

}
