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
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

func TestLocalReadyFunc(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	// Ensure that a successful ready function is executed.
	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
			OutDir:         outDir,
			WaitUntilReady: true,
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	ranReady := false
	ready := func(context.Context) error {
		ranReady = true
		return nil
	}
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, reg, Delegate{
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
	ready = func(context.Context) error { return errors.New(msg) }
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, reg, Delegate{
		Ready: ready,
	}); status != statusError {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusError)
	}
	if s := stderr.String(); !strings.Contains(s, msg) {
		t.Errorf("Local(%+v) didn't write ready error %q to stderr (got %q)", args, msg, s)
	}
}

func TestLocalReadyFuncDisabled(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	// The ready function should be skipped if WaitUntilReady is false.
	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
			OutDir:         outDir,
			WaitUntilReady: false,
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	ranReady := false
	ready := func(context.Context) error {
		ranReady = true
		return nil
	}
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, reg, Delegate{
		Ready: ready,
	}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if ranReady {
		t.Errorf("Local(%+v) ran ready function despite being told not to", args)
	}
}

func TestLocalTestHook(t *gotesting.T) {
	const name = "pkg.Test"
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: name, Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := jsonprotocol.BundleArgs{Mode: jsonprotocol.BundleRunTestsMode, RunTests: &jsonprotocol.BundleRunTestsArgs{OutDir: outDir}}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	var ranPre, ranPost bool
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, reg, Delegate{
		TestHook: func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState) {
			ranPre = true
			return func(context.Context, *testing.TestHookState) {
				ranPost = true
			}
		},
	}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if !ranPre {
		t.Errorf("Local(%+v) didn't run test pre-test hook %q", args, name)
	}
	if !ranPost {
		t.Errorf("Local(%+v) didn't run test post-test hook %q", args, name)
	}
	if len(stderr.String()) != 0 {
		t.Errorf("Local(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}
}

func TestLocalRunHook(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := jsonprotocol.BundleArgs{Mode: jsonprotocol.BundleRunTestsMode, RunTests: &jsonprotocol.BundleRunTestsArgs{OutDir: outDir}}
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}
	var ranPre, ranPost bool
	if status := Local(nil, stdin, &bytes.Buffer{}, &stderr, reg, Delegate{
		RunHook: func(context.Context) (func(context.Context) error, error) {
			ranPre = true
			return func(context.Context) error {
				ranPost = true
				return nil
			}, nil
		},
	}); status != statusSuccess {
		t.Errorf("Local(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if !ranPre {
		t.Errorf("Local(%+v) didn't run test pre-run hook", args)
	}
	if !ranPost {
		t.Errorf("Local(%+v) didn't run test post-run hook", args)
	}
	if len(stderr.String()) != 0 {
		t.Errorf("Local(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}
}
