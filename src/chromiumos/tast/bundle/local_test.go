// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	gotesting "testing"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

func TestLocalRemoteArgs(t *gotesting.T) {
	// Args intended for remote bundles should generate an error when passed to Local.
	args := Args{
		Mode:       RunTestsMode,
		RemoteArgs: RemoteArgs{Target: "user@example.net"},
	}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	if status := Local(stdin, &bytes.Buffer{}, &stderr, nil); status != statusBadArgs {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusBadArgs)
	}
	if len(stderr.String()) == 0 {
		t.Errorf("Local(%+v) didn't write error to stderr", args)
	}
}

func TestLocalBadTest(t *gotesting.T) {
	// A test without a function should trigger a registration error.
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	testing.AddTest(&testing.Test{Name: "pkg.Test"})

	args := Args{Mode: RunTestsMode}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	if status := Local(stdin, &bytes.Buffer{}, &stderr, nil); status != statusBadTests {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusBadTests)
	}
	if len(stderr.String()) == 0 {
		t.Errorf("Local(%+v) didn't write error to stderr", args)
	}
}

func TestLocalRunTest(t *gotesting.T) {
	const name = "pkg.Test"
	ran := false
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	testing.AddTest(&testing.Test{Name: name, Func: func(context.Context, *testing.State) { ran = true }})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := Args{Mode: RunTestsMode, OutDir: outDir}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	if status := Local(stdin, &bytes.Buffer{}, &stderr, nil); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if !ran {
		t.Errorf("Local(%+v) didn't run test %q", args, name)
	}
	if len(stderr.String()) != 0 {
		t.Errorf("Local(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}
}

func TestLocalFaillog(t *gotesting.T) {
	const name = "pkg.Test"
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	testing.AddTest(&testing.Test{Name: name, Func: func(ctx context.Context, s *testing.State) { s.Error("fail") }})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := Args{Mode: RunTestsMode, OutDir: outDir}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	if status := Local(stdin, &bytes.Buffer{}, &stderr, nil); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}

	// ps.txt is saved by faillog.
	p := filepath.Join(outDir, name, "faillog", "ps.txt")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("Local(%+v) didn't save faillog: %v", args, err)
	}
}

func TestLocalReadyFunc(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	testing.AddTest(&testing.Test{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// Ensure that a successful ready function is executed.
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := Args{Mode: RunTestsMode, OutDir: outDir}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	ranReady := false
	ready := func(context.Context, func(string)) error {
		ranReady = true
		return nil
	}
	if status := Local(stdin, &bytes.Buffer{}, &stderr, ready); status != statusSuccess {
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
	if status := Local(stdin, &bytes.Buffer{}, &stderr, ready); status != statusError {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusError)
	}
	if s := stderr.String(); !strings.Contains(s, msg) {
		t.Errorf("Local(%+v) didn't write ready error %q to stderr (got %q)", args, msg, s)
	}
}
