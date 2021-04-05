// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"log"
	"os"
	gotesting "testing"

	"chromiumos/tast/dut"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

func TestRemoteMissingTarget(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// Remote should fail if -target wasn't passed.
	stdin := newBufferWithArgs(t, &BundleArgs{Mode: BundleRunTestsMode, RunTests: &BundleRunTestsArgs{}})
	stderr := bytes.Buffer{}
	if status := Remote(nil, stdin, &bytes.Buffer{}, &stderr, Delegate{}); status != statusError {
		t.Errorf("Remote() = %v; want %v", status, statusError)
	}
	if len(stderr.String()) == 0 {
		t.Error("Remote() didn't write error to stderr")
	}
}

func TestRemoteCantConnect(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// Remote should fail if the initial connection to the DUT couldn't be
	// established since the user key wasn't passed.
	args := BundleArgs{
		Mode:     BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{Target: td.Srvs[0].Addr().String()},
	}
	stderr := bytes.Buffer{}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &stderr, Delegate{}); status != statusError {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusError)
	}
	if len(stderr.String()) == 0 {
		t.Errorf("Remote(%+v) didn't write error to stderr", args)
	}
}

func TestRemoteDUT(t *gotesting.T) {
	const (
		cmd    = "some_command"
		output = "fake output"
	)
	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		if req.Cmd != "exec "+cmd {
			log.Printf("Unexpected command %q", req.Cmd)
			req.Start(false)
		} else {
			req.Start(true)
			req.Write([]byte(output))
			req.End(0)
		}
	})
	defer td.Close()

	// Register a test that runs a command on the DUT and saves its output.
	realOutput := ""
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(ctx context.Context, s *testing.State) {
		dt := s.DUT()
		out, err := dt.Command(cmd).Output(ctx)
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  outDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
		},
	}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &bytes.Buffer{}, Delegate{}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if realOutput != output {
		t.Errorf("Test got output %q from DUT; want %q", realOutput, output)
	}
}

func TestRemoteReconnectBetweenTests(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	// Returns a test function that sets the passed bool to true if the dut.DUT
	// that's passed to the test is connected and then disconnects. This is used
	// to establish that remote bundles reconnect before each test if needed.
	makeFunc := func(conn *bool) func(context.Context, *testing.State) {
		return func(ctx context.Context, s *testing.State) {
			dt := s.DUT()
			*conn = dt.Connected(ctx)
			if err := dt.Disconnect(ctx); err != nil {
				s.Fatal("Failed to disconnect: ", err)
			}
		}
	}

	var conn1, conn2 bool
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: makeFunc(&conn1)})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test2", Func: makeFunc(&conn2)})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  outDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
		},
	}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &bytes.Buffer{}, Delegate{}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if conn1 != true {
		t.Errorf("Remote(%+v) didn't pass live connection to first test", args)
	}
	if conn2 != true {
		t.Errorf("Remote(%+v) didn't pass live connection to second test", args)
	}
}

// TestRemoteTestHooks makes sure hook function is called at the end of a test.
func TestRemoteTestHooks(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	// Add two test instances.
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: func(context.Context, *testing.State) {}})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test2", Func: func(context.Context, *testing.State) {}})

	// Set up test argument.
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  outDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
		},
	}

	// Set up input and output buffers.
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}

	// ranPreHookCount keeps the number of times prehook function was called.
	// ranPostHookCount keepts the number of times posthoook functions was called.
	var ranPreHookCount, ranPostHookCount int

	// Test Remote function.
	if status := Remote(nil, stdin, &bytes.Buffer{}, &stderr, Delegate{
		TestHook: func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState) {
			ranPreHookCount++
			return func(context.Context, *testing.TestHookState) {
				ranPostHookCount++
			}
		},
	}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
	}

	// Make sure prehook function was called twice.
	if ranPreHookCount != 2 {
		t.Errorf("Remote(%+v) test pre hook was called %v times; want 2 times", args, ranPreHookCount)
	}
	// Make sure posthook function was called twice.
	if ranPostHookCount != 2 {
		t.Errorf("Remote(%+v) test post hook was called %v times; want 2 times", args, ranPostHookCount)
	}
	// Make sure there are no unexpected errors from test functions.
	if stderr.String() != "" {
		t.Errorf("Remote(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}

}

// TestBeforeReboot makes sure hook function is called before reboot.
func TestBeforeReboot(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: func(ctx context.Context, s *testing.State) {
		s.DUT().Reboot(ctx)
		s.DUT().Reboot(ctx)
	}})

	// Set up test argument.
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  outDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
		},
	}

	// Set up input and output buffers.
	stdin := newBufferWithArgs(t, &args)
	stderr := bytes.Buffer{}

	// ranBeforeRebootCount keepts the number of times pre-reboot function was called.
	var ranBeforeRebootCount int

	// Test Remote function.
	if status := Remote(nil, stdin, &bytes.Buffer{}, &stderr, Delegate{
		BeforeReboot: func(context.Context, *dut.DUT) error {
			ranBeforeRebootCount++
			return nil
		},
	}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
	}

	// Make sure pre-reboot function was called twice.
	if ranBeforeRebootCount != 2 {
		t.Errorf("Remote(%+v) pre-reboot hook was called %v times; want 2 times", args, ranBeforeRebootCount)
	}
	// Make sure there are no unexpected errors from test functions.
	if stderr.String() != "" {
		t.Errorf("Remote(%+v) unexpectedly wrote %q to stderr", args, stderr.String())
	}
}

// TestRemoteCompanionDUTs make sure we can access companion DUTs.
func TestRemoteCompanionDUTs(t *gotesting.T) {
	const (
		cmd    = "some_command"
		output = "fake output"
	)
	handler := func(req *sshtest.ExecReq) {
		if req.Cmd != "exec "+cmd {
			log.Printf("Unexpected command %q", req.Cmd)
			req.Start(false)
		} else {
			req.Start(true)
			req.Write([]byte(output))
			req.End(0)
		}
	}

	td := sshtest.NewTestData(handler, handler)
	defer td.Close()

	companionHost := td.Srvs[1]

	// Register a test that runs a command on the DUT and saves its output.
	realOutput := ""
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	const role = "role"
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(ctx context.Context, s *testing.State) {
		dt := s.CompanionDUT(role)
		out, err := dt.Command(cmd).Output(ctx)
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:        outDir,
			Target:        td.Srvs[0].Addr().String(),
			CompanionDUTs: map[string]string{role: companionHost.Addr().String()},
			KeyFile:       td.UserKeyFile,
		},
	}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &bytes.Buffer{}, Delegate{}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if realOutput != output {
		t.Errorf("Test got output %q from DUT; want %q", realOutput, output)
	}
}
