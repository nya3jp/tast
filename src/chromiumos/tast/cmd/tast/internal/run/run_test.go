// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/fakereports"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/cmd/tast/internal/run/target"
	frameworkprotocol "chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/devserver/devservertest"
	"chromiumos/tast/internal/faketlw"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

func TestRun(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config()
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.ResDir, runnerclient.ResultsFilename)); err != nil {
		t.Errorf("Results were not saved: %v", err)
	}
}

func TestRunNoTestToRun(t *gotesting.T) {
	// No test in bundles.
	env := runtest.SetUp(t, runtest.WithLocalBundles(testing.NewRegistry("bundle")), runtest.WithRemoteBundles(testing.NewRegistry("bundle")))
	ctx := env.Context()
	cfg := env.Config()
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	// Results are not written in the case no test was run.
	if _, err := os.Stat(filepath.Join(cfg.ResDir, runnerclient.ResultsFilename)); err == nil {
		t.Error("Results were saved despite there was no test to run")
	} else if !os.IsNotExist(err) {
		t.Errorf("Failed to check if results were saved: %v", err)
	}
}

func TestRunPartialRun(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config()
	// Set a nonexistent path for the remote runner so that it will fail.
	cfg.RemoteRunner = filepath.Join(env.TempDir(), "missing_remote_test_runner")
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite missing remote_test_runner")
	}
}

func TestRunError(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config()
	cfg.KeyFile = "" // force SSH auth error
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite unaccessible SSH server")
	}
}

func TestRunEphemeralDevserver(t *gotesting.T) {
	env := runtest.SetUp(t, runtest.WithOnRunLocalTestsInit(func(init *protocol.RunTestsInit) {
		if ds := init.GetRunConfig().GetServiceConfig().GetDevservers(); len(ds) != 1 {
			t.Errorf("Local runner: devservers=%#v; want 1 entry", ds)
		}
	}))
	ctx := env.Context()
	cfg := env.Config()
	cfg.UseEphemeralDevserver = true
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
}

func TestRunDownloadPrivateBundles(t *gotesting.T) {
	ds, err := devservertest.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()

	called := make(map[string]struct{})
	makeHandler := func(role string) runtest.DUTOption {
		return runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			called[role] = struct{}{}
			want := &protocol.ServiceConfig{Devservers: []string{ds.URL}}
			if diff := cmp.Diff(req.GetServiceConfig(), want); diff != "" {
				t.Errorf("ServiceConfig mismatch (-got +want):\n%s", diff)
			}
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		})
	}

	env := runtest.SetUp(
		t,
		makeHandler("dut0"),
		runtest.WithCompanionDUT("dut1", makeHandler("dut1")),
		runtest.WithCompanionDUT("dut2", makeHandler("dut2")),
	)
	ctx := env.Context()
	cfg := env.Config()
	cfg.Devservers = []string{ds.URL}
	cfg.DownloadPrivateBundles = true
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	wantCalled := map[string]struct{}{"dut0": {}, "dut1": {}, "dut2": {}}
	if diff := cmp.Diff(called, wantCalled); diff != "" {
		t.Errorf("DownloadPrivateBundles not called (-got +want):\n%s", diff)
	}
}

func TestRunDownloadPrivateBundlesWithTLW(t *gotesting.T) {
	const gsURL = "gs://a/b/c"

	var tlwAddr string // filled in later
	called := make(map[string]struct{})
	makeHandler := func(role, name string) runtest.DUTOption {
		return runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			called[role] = struct{}{}

			// We should get tlwDUTName as the TLW name.
			// Due to a bug, an incorrect TLW name is given for now.
			// TODO(b/191318903): Uncomment this check once the bug is fixed.
			/*
				if n := req.GetServiceConfig().GetTlwSelfName(); n != name {
					t.Errorf("DownloadPrivateBundles: TLW name mismatch: got %q, want %q", n, name)
				}
			*/

			// DownloadPrivateBundles is called on the DUT, thus we don't have
			// direct access to the TLW server. Tast CLI should have set up SSH
			// port forwarding.
			if addr := req.GetServiceConfig().GetTlwServer(); addr == tlwAddr {
				t.Errorf("DownloadPrivateBundles: TLW is not port-forwarded (%s)", addr)
			}

			// Make sure TLW is working over the forwarded port.
			conn, err := grpc.Dial(req.GetServiceConfig().GetTlwServer(), grpc.WithInsecure())
			if err != nil {
				t.Errorf("DownloadPrivateBundles: Failed to connect to TLW server: %v", err)
				return nil, nil
			}
			defer conn.Close()

			cl := tls.NewWiringClient(conn)
			// TODO(b/191318903): Use name as DutName.
			if _, err = cl.CacheForDut(context.Background(), &tls.CacheForDutRequest{Url: gsURL, DutName: "dut0"}); err != nil {
				t.Errorf("CacheForDut failed: %v", err)
			}
			return nil, nil
		})
	}

	env := runtest.SetUp(
		t,
		makeHandler("primary", "dut0"),
		runtest.WithCompanionDUT("role1", makeHandler("role1", "dut1")),
		runtest.WithCompanionDUT("role2", makeHandler("role2", "dut2")),
	)

	ctx := env.Context()
	cfg := env.Config()

	// Start a TLW server. This needs to be done after runtest.SetUp because
	// the TLW server needs to know the address of the fake SSH server.
	portMap := make(map[faketlw.NamePort]faketlw.NamePort)
	for _, r := range []struct{ name, target string }{
		{"dut0", cfg.Target},
		{"dut1", cfg.CompanionDUTs["role1"]},
		{"dut2", cfg.CompanionDUTs["role2"]},
	} {
		host, portStr, err := net.SplitHostPort(r.target)
		if err != nil {
			t.Fatal("net.SplitHostPort: ", err)
		}
		port, err := strconv.ParseInt(portStr, 10, 32)
		if err != nil {
			t.Fatal("strconv.ParseUint: ", err)
		}
		portMap[faketlw.NamePort{Name: r.name, Port: 22}] = faketlw.NamePort{Name: host, Port: int32(port)}
	}
	stopFunc, tlwAddr := faketlw.StartWiringServer(
		t,
		faketlw.WithDUTName("dut0"),
		faketlw.WithDUTPortMap(portMap),
		faketlw.WithCacheFileMap(map[string][]byte{gsURL: []byte("abc")}),
	)
	defer stopFunc()

	cfg.TLWServer = tlwAddr
	cfg.Target = "dut0"
	cfg.CompanionDUTs["role1"] = "dut1"
	cfg.CompanionDUTs["role2"] = "dut2"
	cfg.DownloadPrivateBundles = true
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	wantCalled := map[string]struct{}{"primary": {}, "role1": {}, "role2": {}}
	if diff := cmp.Diff(called, wantCalled); diff != "" {
		t.Errorf("DownloadPrivateBundles not called (-got +want):\n%s", diff)
	}
}

func TestRunTLW(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config()
	state := env.State()

	host, portStr, err := net.SplitHostPort(cfg.Target)
	if err != nil {
		t.Fatal("net.SplitHostPort: ", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatal("strconv.ParseUint: ", err)
	}

	// Start a TLW server that resolves "the_dut:22" to the real target addr/port.
	const targetName = "the_dut"
	stopFunc, tlwAddr := faketlw.StartWiringServer(t, faketlw.WithDUTPortMap(map[faketlw.NamePort]faketlw.NamePort{
		{Name: targetName, Port: 22}: {Name: host, Port: int32(port)},
	}))
	defer stopFunc()

	cfg.Target = targetName
	cfg.TLWServer = tlwAddr

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
}

// TestRunWithReports_LogStream tests run.Run() with fake Reports server and log stream.
func TestRunWithReports_LogStream(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t, 0)
	defer stopFunc()

	const (
		bundleName   = "bundle"
		test1Name    = "foo.FirstTest"
		test1Path    = "tests/foo.FirstTest/log.txt"
		test1LogText = "Here's a test log message"
		test2Name    = "foo.SecondTest"
		test2Path    = "tests/foo.SecondTest/log.txt"
		test2LogText = "Here's another test log message"
	)

	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    test1Name,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log(test1LogText)
		},
	})
	remoteReg := testing.NewRegistry(bundleName)
	remoteReg.AddTestInstance(&testing.TestInstance{
		Name:    test2Name,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log(test2LogText)
		},
	})

	env := runtest.SetUp(t, runtest.WithLocalBundles(localReg), runtest.WithRemoteBundles(remoteReg))
	ctx := env.Context()
	cfg := env.Config()
	cfg.ReportsServer = addr
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	if str := string(srv.GetLog(test1Name, test1Path)); !strings.Contains(str, test1LogText) {
		t.Errorf("Expected log not received for test 1; got %q; should contain %q", str, test1LogText)
	}
	if str := string(srv.GetLog(test2Name, test2Path)); !strings.Contains(str, test2LogText) {
		t.Errorf("Expected log not received for test 2; got %q; should contain %q", str, test2LogText)
	}
	if str := string(srv.GetLog(test1Name, test1Path)); strings.Contains(str, test2LogText) {
		t.Errorf("Unexpected log found in test 1 log; got %q; should not contain %q", str, test2LogText)
	}
	if str := string(srv.GetLog(test2Name, test2Path)); strings.Contains(str, test1LogText) {
		t.Errorf("Unexpected log found in test 2 log; got %q; should not contain %q", str, test1LogText)
	}
}

// TestRunWithReports_ReportResult tests run.Run() with fake Reports server and reporting results.
func TestRunWithReports_ReportResult(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t, 0)
	defer stopFunc()

	const (
		bundleName  = "bundle"
		test1Name   = "pkg.Test1"
		test2Name   = "pkg.Test2"
		test3Name   = "pkg.Test3"
		test2Error  = "Intentionally failed"
		softwareDep = "swdep"
	)

	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    test1Name,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    test2Name,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error(test2Error)
		},
	})
	localReg.AddTestInstance(&testing.TestInstance{
		Name:         test3Name,
		Timeout:      time.Minute,
		SoftwareDeps: []string{softwareDep},
		Func:         func(ctx context.Context, s *testing.State) {},
	})
	remoteReg := testing.NewRegistry(bundleName)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Unavailable: []string{softwareDep},
						},
					},
				},
			}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config()
	cfg.ReportsServer = addr
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	expectedResults := []*frameworkprotocol.ReportResultRequest{
		{Test: test3Name, SkipReason: "missing SoftwareDeps: swdep"},
		{Test: test1Name},
		{Test: test2Name, Errors: []*frameworkprotocol.ErrorReport{{Reason: test2Error}}},
	}
	results := srv.Results()
	cmpOpt := cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time", "File", "Line", "Stack")
	if diff := cmp.Diff(results, expectedResults, cmpOpt); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}
}

// TestRunWithReports_ReportResultTerminate tests run.Run() stop testing after getting terminate
// response from reports server.
func TestRunWithReports_ReportResultTerminate(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t, 1)
	defer stopFunc()

	const (
		bundleName = "bundle"
		test1Name  = "pkg.Test1"
		test2Name  = "pkg.Test2"
		test3Name  = "pkg.Test3"
		test2Error = "Intentionally failed"
	)

	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    test1Name,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    test2Name,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error(test2Error)
		},
	})
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    test3Name,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	remoteReg := testing.NewRegistry(bundleName)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
	)
	ctx := env.Context()
	cfg := env.Config()
	cfg.ReportsServer = addr
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite termination request")
	}

	expectedResults := []*frameworkprotocol.ReportResultRequest{
		{Test: test1Name},
		{Test: test2Name, Errors: []*frameworkprotocol.ErrorReport{{Reason: test2Error}}},
		// pkg.Test3 is not run.
	}
	results := srv.Results()
	cmpOpt := cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time", "File", "Line", "Stack")
	if diff := cmp.Diff(results, expectedResults, cmpOpt); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}
}

// TestRunWithSkippedTests makes sure that tests with unsupported dependency
// would be skipped.
func TestRunWithSkippedTests(t *gotesting.T) {
	const (
		bundleName     = "bundle"
		localTestName  = "pkg.LocalTest"
		remoteTestName = "pkg.RemoteTest"
		softwareDep    = "swdep"
	)

	// Both of two tests depends on a missing software feature, thus they
	// are skipped.

	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:         localTestName,
		Timeout:      time.Minute,
		SoftwareDeps: []string{softwareDep},
		Func: func(ctx context.Context, s *testing.State) {
			t.Errorf("%s was run despite unsatisfied dependency", localTestName)
		},
	})

	remoteReg := testing.NewRegistry(bundleName)
	remoteReg.AddTestInstance(&testing.TestInstance{
		Name:         remoteTestName,
		Timeout:      time.Minute,
		SoftwareDeps: []string{softwareDep},
		Func: func(ctx context.Context, s *testing.State) {
			t.Errorf("%s was run despite unsatisfied dependency", remoteTestName)
		},
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Unavailable: []string{softwareDep},
						},
					},
				},
			}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config()
	state := env.State()

	results, err := run.Run(ctx, cfg, state)
	if err != nil {
		t.Errorf("Run failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Got %d results; want %d", len(results), 2)
	}
}

// TestListTests make sure list test can list all tests.
func TestRunListTests(t *gotesting.T) {
	const (
		bundleName = "bundle"

		localTestName   = "pkg.LocalTest"
		remoteTestName  = "pkg.RemoteTest"
		skippedTestName = "pkg.SkippedTest"

		missingSoftwareDep = "missing"
	)

	localTest := &testing.TestInstance{
		Name:    localTestName,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(localTest)

	remoteTest := &testing.TestInstance{
		Name:    remoteTestName,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	skippedTest := &testing.TestInstance{
		Name:         skippedTestName,
		Timeout:      time.Minute,
		SoftwareDeps: []string{missingSoftwareDep},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	remoteReg := testing.NewRegistry(bundleName)
	remoteReg.AddTestInstance(remoteTest)
	remoteReg.AddTestInstance(skippedTest)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Unavailable: []string{missingSoftwareDep},
						},
					},
				},
			}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config()
	cfg.Mode = config.ListTestsMode
	state := env.State()

	results, err := run.Run(ctx, cfg, state)
	if err != nil {
		t.Errorf("Run failed: %v", err)
	}

	localTestMeta, _ := resultsjson.NewTest(localTest.EntityProto())
	remoteTestMeta, _ := resultsjson.NewTest(remoteTest.EntityProto())
	skippedTestMeta, _ := resultsjson.NewTest(skippedTest.EntityProto())
	expected := []*resultsjson.Result{
		{Test: *skippedTestMeta, SkipReason: "missing SoftwareDeps: missing"},
		{Test: *localTestMeta},
		{Test: *remoteTestMeta},
	}
	if diff := cmp.Diff(results, expected); diff != "" {
		t.Errorf("Unexpected results (-got +want):\n%s", diff)
	}
}

// TestRunListTestsWithSharding make sure list test can list tests in specified shards.
func TestRunListTestsWithSharding(t *gotesting.T) {
	const (
		bundleName = "bundle"

		localTestName   = "pkg.LocalTest"
		remoteTestName  = "pkg.RemoteTest"
		skippedTestName = "pkg.SkippedTest"

		missingSoftwareDep = "missing"
	)

	localTest := &testing.TestInstance{
		Name:    localTestName,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(localTest)

	remoteTest := &testing.TestInstance{
		Name:    remoteTestName,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	skippedTest := &testing.TestInstance{
		Name:         skippedTestName,
		Timeout:      time.Minute,
		SoftwareDeps: []string{missingSoftwareDep},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	remoteReg := testing.NewRegistry(bundleName)
	remoteReg.AddTestInstance(remoteTest)
	remoteReg.AddTestInstance(skippedTest)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Unavailable: []string{missingSoftwareDep},
						},
					},
				},
			}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config()
	cfg.Mode = config.ListTestsMode
	cfg.TotalShards = 2 // set the number of shards
	state := env.State()

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(ctx)

	localTestMeta, _ := resultsjson.NewTest(localTest.EntityProto())
	remoteTestMeta, _ := resultsjson.NewTest(remoteTest.EntityProto())
	skippedTestMeta, _ := resultsjson.NewTest(skippedTest.EntityProto())

	for shardIndex, expected := range [][]*resultsjson.Result{
		{
			{Test: *skippedTestMeta, SkipReason: "missing SoftwareDeps: missing"},
			{Test: *localTestMeta},
		},
		{
			{Test: *remoteTestMeta},
		},
	} {
		t.Run(fmt.Sprintf("shard%d", shardIndex), func(t *gotesting.T) {
			cfg.ShardIndex = shardIndex

			results, err := run.Run(ctx, cfg, state)
			if err != nil {
				t.Errorf("Run failed: %v", err)
			}

			if diff := cmp.Diff(results, expected); diff != "" {
				t.Errorf("Unexpected results (-got +want):\n%s", diff)
			}
		})
	}
}

func TestRunPrintOSVersion(t *gotesting.T) {
	const osVersion = "octopus-release/R86-13312.0.2020_07_02_1108"

	env := runtest.SetUp(t, runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
		return &protocol.GetDUTInfoResponse{
			DutInfo: &protocol.DUTInfo{
				Features: &protocol.DUTFeatures{
					Software: &protocol.SoftwareFeatures{
						// Must report non-empty features.
						// TODO(b/187793617): Remove this once we fully migrate to the gRPC protocol and
						// GetDUTInfo gets capable of returning errors.
						Available: []string{"foo"},
					},
				},
				OsVersion: osVersion,
			},
		}, nil
	}))
	ctx := env.Context()
	logger := loggingtest.NewLogger(t, logging.LevelInfo)
	ctx = logging.AttachLoggerNoPropagation(ctx, logger)
	cfg := env.Config()
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	const expectedOSVersion = "Target version: " + osVersion
	if logs := logger.String(); !strings.Contains(logs, expectedOSVersion) {
		t.Errorf("Cannot find %q in log buffer %q", expectedOSVersion, logs)
	}
}

func TestRunCollectSysInfo(t *gotesting.T) {
	initState := &protocol.SysInfoState{
		LogInodeSizes: map[uint64]int64{
			12: 34,
			56: 78,
		},
	}
	called := false

	env := runtest.SetUp(t,
		runtest.WithGetSysInfoState(func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
			return &protocol.GetSysInfoStateResponse{State: initState}, nil
		}),
		runtest.WithCollectSysInfo(func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
			called = true
			if diff := cmp.Diff(req.GetInitialState(), initState); diff != "" {
				t.Errorf("SysInfoState mismatch (-got +want):\n%s", diff)
			}
			return &protocol.CollectSysInfoResponse{}, nil
		}))
	ctx := env.Context()
	cfg := env.Config()
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	if !called {
		t.Error("CollectSysInfo was not called")
	}
}
