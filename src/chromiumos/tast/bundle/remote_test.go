// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"crypto/rsa"
	"os"
	gotesting "testing"

	"chromiumos/tast/dut"
	"chromiumos/tast/host/test"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = test.MustGenerateKeys()
}

func TestRemoteMissingTarget(t *gotesting.T) {
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: "pkg.Test", Func: func(s *testing.State) {}})

	// Remote should fail if -target wasn't passed.
	args := []string{"-report"}
	if act, exp := Remote(args), statusBadArgs; act != exp {
		t.Errorf("Remote(%v) = %v; want %v", args, act, exp)
	}
}

func TestRemoteCantConnect(t *gotesting.T) {
	td := test.NewTestData(userKey, hostKey)
	defer td.Close()

	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: "pkg.Test", Func: func(s *testing.State) {}})

	// Remote should fail if the initial connection to the DUT couldn't be
	// established since the user key wasn't passed.
	args := []string{"-target=" + td.Srv.Addr().String()}
	if act, exp := Remote(args), statusError; act != exp {
		t.Errorf("Remote(%v) = %v; want %v", args, act, exp)
	}
}

func TestRemoteDUT(t *gotesting.T) {
	td := test.NewTestData(userKey, hostKey)
	defer td.Close()

	const (
		cmd    = "some_command"
		output = "fake output"
	)
	td.Srv.FakeCmd(cmd, 0, []byte(output), []byte{}, nil)

	// Register a test that runs a command on the DUT and saves its output.
	realOutput := ""
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: "pkg.Test", Func: func(s *testing.State) {
		dt, ok := dut.FromContext(s.Context())
		if !ok {
			s.Fatal("Failed to get DUT from context")
		}
		out, err := dt.Run(s.Context(), cmd)
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	outDir := testutil.TempDir(t, "bundle_test.")
	defer os.RemoveAll(outDir)
	args := []string{"-keyfile=" + td.UserKeyFile,
		"-target=" + td.Srv.Addr().String(), "-outdir=" + outDir}
	if act, exp := Remote(args), statusSuccess; act != exp {
		t.Errorf("Remote(%v) = %v; want %v", args, act, exp)
	}
	if realOutput != output {
		t.Errorf("Test got output %q from DUT; want %q", realOutput, output)
	}
}

func TestRemoteReconnectBetweenTests(t *gotesting.T) {
	td := test.NewTestData(userKey, hostKey)
	defer td.Close()

	// Returns a test function that sets the passed bool to true if the dut.DUT
	// that's passed to the test is connected and then disconnects. This is used
	// to establish that remote bundles reconnect before each test if needed.
	makeFunc := func(conn *bool) func(*testing.State) {
		return func(s *testing.State) {
			dt, ok := dut.FromContext(s.Context())
			if !ok {
				s.Fatal("Failed to get DUT from context")
			}
			*conn = dt.Connected(s.Context())
			if err := dt.Disconnect(s.Context()); err != nil {
				s.Fatal("Failed to disconnect: ", err)
			}
		}
	}

	var conn1, conn2 bool
	defer testing.ClearForTesting()
	testing.GlobalRegistry().DisableValidationForTesting()
	testing.AddTest(&testing.Test{Name: "pkg.Test1", Func: makeFunc(&conn1)})
	testing.AddTest(&testing.Test{Name: "pkg.Test2", Func: makeFunc(&conn2)})

	outDir := testutil.TempDir(t, "bundle_test.")
	defer os.RemoveAll(outDir)
	args := []string{"-keyfile=" + td.UserKeyFile,
		"-target=" + td.Srv.Addr().String(), "-outdir=" + outDir}
	if act, exp := Remote(args), statusSuccess; act != exp {
		t.Errorf("Remote(%v) = %v; want %v", args, act, exp)
	}
	if conn1 != true {
		t.Errorf("Remote(%v) didn't pass live connection to first test", args)
	}
	if conn2 != true {
		t.Errorf("Remote(%v) didn't pass live connection to second test", args)
	}
}
