// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/protocol/protocoltest"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

// startTestServer starts an in-process gRPC server and returns a connection as
// TestServiceClient. On completion of the current test, resources are released
// automatically.
func startTestServer(t *gotesting.T, scfg *StaticConfig, req *protocol.HandshakeRequest) protocol.TestServiceClient {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	t.Cleanup(func() {
		cw.Close()
		cr.Close()
	})

	go run(context.Background(), []string{"-rpc"}, sr, sw, ioutil.Discard, scfg)

	conn, err := rpc.NewClient(context.Background(), cr, cw, req)
	if err != nil {
		t.Fatalf("Failed to connect to in-process gRPC server: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	return protocol.NewTestServiceClient(conn.Conn())
}

var testFunc = func(context.Context, *testing.State) {}

// testPre implements Precondition for unit tests.
// TODO(derat): This is duplicated from tast/testing/test_test.go. Find a common location.
type testPre struct {
	prepareFunc func(context.Context, *testing.PreState) interface{}
	closeFunc   func(context.Context, *testing.PreState)
	name        string // name to return from String
}

func (p *testPre) Prepare(ctx context.Context, s *testing.PreState) interface{} {
	if p.prepareFunc != nil {
		return p.prepareFunc(ctx, s)
	}
	return nil
}

func (p *testPre) Close(ctx context.Context, s *testing.PreState) {
	if p.closeFunc != nil {
		p.closeFunc(ctx, s)
	}
}

func (p *testPre) Timeout() time.Duration { return time.Minute }

func (p *testPre) String() string { return p.name }

func TestRunTests(t *gotesting.T) {
	const (
		name1       = "foo.Test1"
		name2       = "foo.Test2"
		preRunMsg   = "setting up for run"
		postRunMsg  = "cleaning up after run"
		preTestMsg  = "setting up for test"
		postTestMsg = "cleaning up for test"
		errorMsg    = "error"
	)

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Name:    name1,
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute},
	)
	reg.AddTestInstance(&testing.TestInstance{
		Name:    name2,
		Func:    func(ctx context.Context, s *testing.State) { s.Error(errorMsg) },
		Timeout: time.Minute},
	)

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	runTmpDir := filepath.Join(tmpDir, "run_tmp")
	if err := os.Mkdir(runTmpDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", runTmpDir, err)
	}
	if err := ioutil.WriteFile(filepath.Join(runTmpDir, "foo.txt"), nil, 0644); err != nil {
		t.Fatalf("Failed to create foo.txt: %v", err)
	}

	var preRunCalls, postRunCalls, preTestCalls, postTestCalls int
	cfg := &protocol.RunConfig{
		Tests: []string{name1, name2},
		Dirs: &protocol.RunDirectories{
			OutDir:  tmpDir,
			DataDir: tmpDir,
			TempDir: runTmpDir,
		},
	}
	scfg := NewStaticConfig(reg, 0, Delegate{
		RunHook: func(ctx context.Context) (func(context.Context) error, error) {
			preRunCalls++
			logging.Info(ctx, preRunMsg)
			return func(ctx context.Context) error {
				postRunCalls++
				logging.Info(ctx, postRunMsg)
				return nil
			}, nil
		},
		TestHook: func(ctx context.Context, s *testing.TestHookState) func(ctx context.Context, s *testing.TestHookState) {
			preTestCalls++
			s.Log(preTestMsg)

			return func(ctx context.Context, s *testing.TestHookState) {
				postTestCalls++
				s.Log(postTestMsg)
			}
		},
	})

	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})

	events, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg,
		protocoltest.WithRunLogs(),
		protocoltest.WithEntityLogs(),
	)
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	if preRunCalls != 1 {
		t.Errorf("RunTests called pre-run function %d time(s); want 1", preRunCalls)
	}
	if postRunCalls != 1 {
		t.Errorf("RunTests called run post-run function %d time(s); want 1", postRunCalls)
	}

	tests := reg.AllTests()
	if preTestCalls != len(tests) {
		t.Errorf("RunTests called pre-test function %d time(s); want %d", preTestCalls, len(tests))
	}
	if postTestCalls != len(tests) {
		t.Errorf("RunTests called post-test function %d time(s); want %d", postTestCalls, len(tests))
	}

	// Just check some basic details of the control messages.
	wantEvents := []protocol.Event{
		&protocol.RunLogEvent{Text: preRunMsg},
		&protocol.RunLogEvent{Text: "Devserver status: using pseudo client"},
		&protocol.RunLogEvent{Text: "Found 0 external linked data file(s), need to download 0"},
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: name1, Text: preTestMsg},
		&protocol.EntityLogEvent{EntityName: name1, Text: postTestMsg},
		&protocol.EntityEndEvent{EntityName: name1},
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: name2, Text: preTestMsg},
		&protocol.EntityErrorEvent{EntityName: name2, Error: &protocol.Error{Reason: errorMsg}},
		&protocol.EntityLogEvent{EntityName: name2, Text: postTestMsg},
		&protocol.EntityEndEvent{EntityName: name2},
		&protocol.RunLogEvent{Text: postRunMsg},
	}
	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	// RunTests should report success when no test is executed.
	cl := startTestServer(t, NewStaticConfig(testing.NewRegistry("bundle"), 0, Delegate{}), &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, &protocol.RunConfig{}); err != nil {
		t.Fatalf("RunTests failed for empty tests: %v", err)
	}
}

func TestRunTestsRemoteData(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	reg := testing.NewRegistry("bundle")

	var (
		meta *testing.Meta
		hint *testing.RPCHint
		dt   *dut.DUT
	)
	reg.AddTestInstance(&testing.TestInstance{
		Name: "meta.Test",
		Func: func(ctx context.Context, s *testing.State) {
			meta = s.Meta()
			hint = s.RPCHint()
			dt = s.DUT()
		},
	})

	cfg := &protocol.RunConfig{
		Features: &protocol.Features{
			Infra: &protocol.InfraFeatures{
				Vars: map[string]string{"var1": "value1"},
			},
		},
	}
	hr := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					DutConfig: &protocol.DUTConfig{
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[0].Addr().String(),
							KeyFile:        td.UserKeyFile,
						},
					},
					BundleDir: "/mock/local/bundles",
				},
				MetaTestConfig: &protocol.MetaTestConfig{
					TastPath: "/bogus/tast",
					RunFlags: []string{"-flag1", "-flag2"},
				},
			},
		},
	}

	cl := startTestServer(t, NewStaticConfig(reg, time.Minute, Delegate{}), hr)
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	// The test should have access to information related to remote tests.
	expMeta := &testing.Meta{
		Target:         hr.BundleInitParams.BundleConfig.PrimaryTarget.DutConfig.SshConfig.ConnectionSpec,
		TastPath:       hr.BundleInitParams.BundleConfig.MetaTestConfig.TastPath,
		RunFlags:       hr.BundleInitParams.BundleConfig.MetaTestConfig.RunFlags,
		ConnectionSpec: hr.BundleInitParams.BundleConfig.PrimaryTarget.DutConfig.SshConfig.ConnectionSpec,
	}
	if diff := cmp.Diff(meta, expMeta); diff != "" {
		t.Errorf("Meta mismtach; (-got +want)\n%v", diff)
	}
	expHint := testing.NewRPCHint(hr.BundleInitParams.BundleConfig.PrimaryTarget.BundleDir, cfg.Features.Infra.Vars)
	if !reflect.DeepEqual(hint, expHint) {
		t.Errorf("Test got RPCHint %+v; want %+v", *hint, *expHint)
	}
	if dt == nil {
		t.Error("DUT is not available")
	}
}

func TestRunTestsOutDir(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	outDir := filepath.Join(td, "out")

	cl := startTestServer(t, NewStaticConfig(testing.NewRegistry("bundle"), 0, Delegate{}), &protocol.HandshakeRequest{})
	cfg := &protocol.RunConfig{
		Dirs: &protocol.RunDirectories{
			OutDir: outDir,
		},
	}
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	// OutDir is created by RunTests.
	fi, err := os.Stat(outDir)
	if err != nil {
		t.Fatalf("Failed to stat output directory: %v", err)
	}

	// OutDir should be writable.
	const wantPerm = 0755
	if perm := fi.Mode().Perm(); perm != wantPerm {
		t.Errorf("Unexpected output directory permission: got 0%o, want 0%o", perm, wantPerm)
	}
}

func TestRunTestsStartFixture(t *gotesting.T) {
	const testName = "pkg.Test"
	// runTests should not run runHook if tests depend on remote fixtures.
	// TODO(crbug/1184567): consider long term plan about interactions between
	// remote fixtures and run hooks.
	cfg := &protocol.RunConfig{
		Tests:             []string{testName},
		StartFixtureState: &protocol.StartFixtureState{Name: "foo"},
	}
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Fixture: "foo",
		Name:    testName,
		Func:    func(context.Context, *testing.State) {},
	})
	scfg := NewStaticConfig(reg, 0, Delegate{
		RunHook: func(context.Context) (func(context.Context) error, error) {
			t.Error("runHook unexpectedly called")
			return nil, nil
		},
	})
	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	// If StartFixtureName is empty, runHook should run.
	cfg = &protocol.RunConfig{
		Tests:             []string{testName},
		StartFixtureState: &protocol.StartFixtureState{Name: ""},
	}
	called := false
	scfg = NewStaticConfig(reg, 0, Delegate{
		RunHook: func(context.Context) (func(context.Context) error, error) {
			called = true
			return nil, nil
		},
	})
	cl = startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if !called {
		t.Error("runHook was not called")
	}
}
func TestRunTestsReadyFuncSystemServiceTimeoutCfgSet(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	expectedSystemServiceTimeout := time.Second * 3
	cfg := &protocol.RunConfig{
		WaitUntilReady:        true,
		SystemServicesTimeout: durationpb.New(expectedSystemServiceTimeout),
	}
	var actualSystemServiceTimeout time.Duration
	scfg := NewStaticConfig(reg, time.Minute, Delegate{
		Ready: func(ctx context.Context, systemServiceTimeout time.Duration) error {
			actualSystemServiceTimeout = systemServiceTimeout
			return nil
		},
	})
	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	if actualSystemServiceTimeout != expectedSystemServiceTimeout {
		t.Fatalf("Expecting SystemServiceTimeout to be %f seconds, however it is %f seconds", expectedSystemServiceTimeout.Seconds(), actualSystemServiceTimeout.Seconds())
	}
}
func TestRunTestsReadyFunc(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// Ensure that a successful ready function is executed.
	cfg := &protocol.RunConfig{
		WaitUntilReady: true,
	}
	ranReady := false
	scfg := NewStaticConfig(reg, time.Minute, Delegate{
		Ready: func(context.Context, time.Duration) error {
			ranReady = true
			return nil
		},
	})
	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if !ranReady {
		t.Error("RunTests didn't run ready function")
	}

	// RunTests should fail if the ready function returns an error.
	const msg = "intentional failure"
	scfg = NewStaticConfig(reg, time.Minute, Delegate{
		Ready: func(context.Context, time.Duration) error { return errors.New(msg) },
	})
	cl = startTestServer(t, scfg, &protocol.HandshakeRequest{})
	_, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg)
	if err == nil {
		t.Fatal("RunTests unexpectedly succeeded despite ready hook failure")
	}
	if s := err.Error(); !strings.Contains(s, msg) {
		t.Errorf("RunTests error doesn't include error message %q: %v", msg, s)
	}
}

func TestRunTestsReadyFuncDisabled(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// The ready function should be skipped if WaitUntilReady is false.
	cfg := &protocol.RunConfig{
		WaitUntilReady: false,
	}
	ranReady := false
	scfg := NewStaticConfig(reg, time.Minute, Delegate{
		Ready: func(context.Context, time.Duration) error {
			ranReady = true
			return nil
		},
	})
	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if ranReady {
		t.Error("RunTests ran ready function despite being told not to")
	}
}

func TestRunTestsTestHook(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	cfg := &protocol.RunConfig{}
	var ranPre, ranPost bool
	scfg := NewStaticConfig(reg, time.Minute, Delegate{
		TestHook: func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState) {
			ranPre = true
			return func(context.Context, *testing.TestHookState) {
				ranPost = true
			}
		},
	})
	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if !ranPre {
		t.Error("RunTests didn't run pre-test hook")
	}
	if !ranPost {
		t.Error("RunTests didn't run post-test hook")
	}
}

func TestRunTestsRunHook(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	cfg := &protocol.RunConfig{}
	var ranPre, ranPost bool
	scfg := NewStaticConfig(reg, time.Minute, Delegate{
		RunHook: func(context.Context) (func(context.Context) error, error) {
			ranPre = true
			return func(context.Context) error {
				ranPost = true
				return nil
			}, nil
		},
	})
	cl := startTestServer(t, scfg, &protocol.HandshakeRequest{})
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if !ranPre {
		t.Error("RunTests didn't run pre-run hook")
	}
	if !ranPost {
		t.Error("RunTests didn't run post-run hook")
	}
}

func TestRunTestsRemoteCantConnect(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// RunTests should fail if the initial connection to the DUT couldn't be
	// established since the user key wasn't passed.
	cfg := &protocol.RunConfig{}
	hr := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					DutConfig: &protocol.DUTConfig{
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[0].Addr().String(),
							// KeyFile is missing.
						},
					},
				},
			},
		},
	}

	cl := startTestServer(t, NewStaticConfig(reg, time.Minute, Delegate{}), hr)
	_, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg)
	if err == nil {
		t.Fatal("RunTests unexpectedly succeeded despite unconnectable server")
	}
	const msg = "failed to connect to DUT"
	if s := err.Error(); !strings.Contains(s, msg) {
		t.Errorf("RunTests error doesn't include error message %q: %v", msg, s)
	}
}

func TestRunTestsRemoteDUT(t *gotesting.T) {
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
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(ctx context.Context, s *testing.State) {
		dt := s.DUT()
		out, err := dt.Conn().CommandContext(ctx, cmd).Output()
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	hr := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					DutConfig: &protocol.DUTConfig{
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[0].Addr().String(),
							KeyFile:        td.UserKeyFile,
						},
					},
				},
			},
		},
	}

	cfg := &protocol.RunConfig{}
	cl := startTestServer(t, NewStaticConfig(reg, time.Minute, Delegate{}), hr)
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	if realOutput != output {
		t.Errorf("Test got output %q from DUT; want %q", realOutput, output)
	}
}

func TestRunTestsRemoteReconnectBetweenTests(t *gotesting.T) {
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
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: makeFunc(&conn1)})
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test2", Func: makeFunc(&conn2)})

	cfg := &protocol.RunConfig{}
	hr := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					DutConfig: &protocol.DUTConfig{
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[0].Addr().String(),
							KeyFile:        td.UserKeyFile,
						},
					},
				},
			},
		},
	}

	cl := startTestServer(t, NewStaticConfig(reg, time.Minute, Delegate{}), hr)
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if !conn1 {
		t.Error("RunTests didn't pass live connection to first test")
	}
	if !conn2 {
		t.Error("RunTests didn't pass live connection to second test")
	}
}

// TestRunTestsRemoteBeforeReboot makes sure hook function is called before reboot.
func TestRunTestsRemoteBeforeReboot(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: func(ctx context.Context, s *testing.State) {
		s.DUT().Reboot(ctx)
		s.DUT().Reboot(ctx)
	}})

	cfg := &protocol.RunConfig{}
	hr := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					DutConfig: &protocol.DUTConfig{
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[0].Addr().String(),
							KeyFile:        td.UserKeyFile,
						},
					},
				},
			},
		},
	}

	ranBeforeRebootCount := 0
	scfg := NewStaticConfig(reg, time.Minute, Delegate{
		BeforeReboot: func(context.Context, *dut.DUT) error {
			ranBeforeRebootCount++
			return nil
		},
	})

	cl := startTestServer(t, scfg, hr)
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	// Make sure pre-reboot function was called twice.
	if ranBeforeRebootCount != 2 {
		t.Errorf("RunTests called pre-reboot hook %v times; want 2 times", ranBeforeRebootCount)
	}
}

// TestRunTestsRemoteCompanionDUTs make sure we can access companion DUTs.
func TestRunTestsRemoteCompanionDUTs(t *gotesting.T) {
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

	// Register a test that runs a command on the DUT and saves its output.
	realOutput := ""
	reg := testing.NewRegistry("bundle")
	const role = "role"
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(ctx context.Context, s *testing.State) {
		dt := s.CompanionDUT(role)
		out, err := dt.Conn().CommandContext(ctx, cmd).Output()
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	cfg := &protocol.RunConfig{}
	hr := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					DutConfig: &protocol.DUTConfig{
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[0].Addr().String(),
							KeyFile:        td.UserKeyFile,
						},
					},
				},
				CompanionDuts: map[string]*protocol.DUTConfig{
					role: {
						SshConfig: &protocol.SSHConfig{
							ConnectionSpec: td.Srvs[1].Addr().String(),
							KeyFile:        td.UserKeyFile,
						},
					},
				},
			},
		},
	}

	cl := startTestServer(t, NewStaticConfig(reg, time.Minute, Delegate{}), hr)
	if _, err := protocoltest.RunTestsForEvents(context.Background(), cl, cfg); err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}
	if realOutput != output {
		t.Errorf("Test got output %q from DUT; want %q", realOutput, output)
	}
}

func TestTestsToRunSortTests(t *gotesting.T) {
	const (
		test1 = "pkg.Test1"
		test2 = "pkg.Test2"
		test3 = "pkg.Test3"
	)

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: test2, Func: testFunc})
	reg.AddTestInstance(&testing.TestInstance{Name: test3, Func: testFunc})
	reg.AddTestInstance(&testing.TestInstance{Name: test1, Func: testFunc})

	tests, err := testsToRun(NewStaticConfig(reg, 0, Delegate{}), nil)
	if err != nil {
		t.Fatal("testsToRun failed: ", err)
	}

	var act []string
	for _, t := range tests {
		act = append(act, t.Name)
	}
	if exp := []string{test1, test2, test3}; !reflect.DeepEqual(act, exp) {
		t.Errorf("testsToRun() returned tests %v; want sorted %v", act, exp)
	}
}

func TestTestsToRunTestTimeouts(t *gotesting.T) {
	const (
		name1          = "pkg.Test1"
		name2          = "pkg.Test2"
		customTimeout  = 45 * time.Second
		defaultTimeout = 30 * time.Second
	)

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{Name: name1, Func: testFunc, Timeout: customTimeout})
	reg.AddTestInstance(&testing.TestInstance{Name: name2, Func: testFunc})

	tests, err := testsToRun(NewStaticConfig(reg, defaultTimeout, Delegate{}), nil)
	if err != nil {
		t.Fatal("testsToRun failed: ", err)
	}

	act := make(map[string]time.Duration, len(tests))
	for _, t := range tests {
		act[t.Name] = t.Timeout
	}
	exp := map[string]time.Duration{name1: customTimeout, name2: defaultTimeout}
	if !reflect.DeepEqual(act, exp) {
		t.Errorf("Wanted tests/timeouts %v; got %v", act, exp)
	}
}

func TestPrepareTempDir(t *gotesting.T) {
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	if err := testutil.WriteFiles(tmpDir, map[string]string{
		"existing.txt": "foo",
	}); err != nil {
		t.Fatal("Failed to create initial files: ", err)
	}

	origTmpDir := os.Getenv("TMPDIR")

	restore, err := prepareTempDir(tmpDir)
	if err != nil {
		t.Fatal("prepareTempDir failed: ", err)
	}
	defer func() {
		if restore != nil {
			restore()
		}
	}()

	if env := os.Getenv("TMPDIR"); env != tmpDir {
		t.Errorf("$TMPDIR = %q; want %q", env, tmpDir)
	}

	fi, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatal("Stat failed: ", err)
	}

	const exp = 0777
	if perm := fi.Mode().Perm(); perm != exp {
		t.Errorf("Incorrect $TMPDIR permission: got %o, want %o", perm, exp)
	}
	if fi.Mode()&os.ModeSticky == 0 {
		t.Error("Incorrect $TMPDIR permission: sticky bit not set")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "existing.txt")); err != nil {
		t.Error("prepareTempDir should not clobber the directory: ", err)
	}

	restore()
	restore = nil

	if env := os.Getenv("TMPDIR"); env != origTmpDir {
		t.Errorf("restore did not restore $TMPDIR; got %q, want %q", env, origTmpDir)
	}

	if _, err := os.Stat(tmpDir); err != nil {
		t.Error("restore must preserve the temporary directory: ", err)
	}
}
