// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/run/fakereports"
	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/errors"
	frameworkprotocol "chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/faketlw"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
)

func TestRun(t *gotesting.T) {
	ctx := context.Background()

	env := runtest.SetUp(t)
	cfg := env.Config()
	state := env.State()

	if _, err := Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.ResDir, runnerclient.ResultsFilename)); err != nil {
		t.Errorf("Results were not saved: %v", err)
	}
}

func TestRunNoTestToRun(t *gotesting.T) {
	ctx := context.Background()

	// No test in bundles.
	env := runtest.SetUp(t, runtest.WithLocalBundle(testing.NewRegistry()), runtest.WithRemoteBundle(testing.NewRegistry()))
	cfg := env.Config()
	state := env.State()

	if _, err := Run(ctx, cfg, state); err != nil {
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
	ctx := context.Background()

	env := runtest.SetUp(t)
	cfg := env.Config()
	// Set a nonexistent path for the remote runner so that it will fail.
	cfg.RemoteRunner = filepath.Join(env.TempDir(), "missing_remote_test_runner")
	state := env.State()

	if _, err := Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite missing remote_test_runner")
	}
}

func TestRunError(t *gotesting.T) {
	ctx := context.Background()

	env := runtest.SetUp(t)
	cfg := env.Config()
	cfg.KeyFile = "" // force SSH auth error
	state := env.State()

	if _, err := Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite unaccessible SSH server")
	}
}

func TestRunEphemeralDevserver(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.Cfg.RunLocal = true
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			if len(args.RunTests.Devservers) != 1 {
				t.Errorf("Devservers=%#v; want 1 entry", args.RunTests.Devservers)
			}
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, nil)
		}
		return 0
	}
	td.Cfg.Devservers = nil // clear the default mock devservers set in NewLocalTestData
	td.Cfg.UseEphemeralDevserver = true

	if _, err := Run(context.Background(), &td.Cfg, &td.State); err != nil {
		t.Errorf("Run failed: %v", err)
	}
}

func TestRunDownloadPrivateBundles(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()
	td.Cfg.Devservers = []string{"http://example.com:8080"}
	testRunDownloadPrivateBundles(t, td)
}

func TestRunDownloadPrivateBundlesWithTLW(t *gotesting.T) {
	const targetName = "dut001"
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	host, portStr, err := net.SplitHostPort(td.Cfg.Target)
	if err != nil {
		t.Fatal("net.SplitHostPort: ", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatal("strconv.ParseUint: ", err)
	}

	// Start a TLW server that resolves "dut001:22" to the real target addr/port.
	stopFunc, tlwAddr := faketlw.StartWiringServer(t, faketlw.WithDUTPortMap(map[faketlw.NamePort]faketlw.NamePort{
		{Name: targetName, Port: 22}: {Name: host, Port: int32(port)},
	}), faketlw.WithCacheFileMap(map[string][]byte{"gs://a/b/c": []byte("abc")}),
		faketlw.WithDUTName(targetName))
	defer stopFunc()

	td.Cfg.Target = targetName
	td.Cfg.TLWServer = tlwAddr
	td.Cfg.Devservers = nil

	testRunDownloadPrivateBundles(t, td)
}

func checkTLWServer(address string) error {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return errors.Wrapf(err, "failed to dial to %s", address)
	}
	defer conn.Close()
	req := tls.CacheForDutRequest{Url: "gs://a/b/c", DutName: "dut001"}
	cl := tls.NewWiringClient(conn)
	ctx := context.Background()
	if _, err = cl.CacheForDut(ctx, &req); err != nil {
		return errors.Wrapf(err, "failed to call CacheForDut(%v)", req)
	}
	return nil
}

// TestRunDownloadPrivateBundlesWithCompanionDUTs makes sure private bundles would be downloaded.
func TestRunDownloadPrivateBundlesWithCompanionDUTs(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t, fakerunner.WithCompanionDUTRoles([]string{"cd1", "cd2"}))
	defer td.Close()
	td.Cfg.Devservers = []string{"http://example.com:8080"}
	testRunDownloadPrivateBundles(t, td)
}

// TestRunDownloadPrivateBundlesWithCompanionDUTsAndTLW makes sure private bundles would be downloaded with TLw.
func TestRunDownloadPrivateBundlesWithCompanionDUTsAndTLW(t *gotesting.T) {
	const targetName = "dut001"
	const companionRole = "compRole"
	const companionDutName = "compDUT0"
	td := fakerunner.NewLocalTestData(t, fakerunner.WithCompanionDUTRoles([]string{companionRole}))
	defer td.Close()

	var hosts []string
	var ports []uint64
	for _, server := range td.SrvData.Srvs {
		host, portStr, err := net.SplitHostPort(server.Addr().String())
		if err != nil {
			t.Fatal("net.SplitHostPort: ", err)
		}
		hosts = append(hosts, host)
		port, err := strconv.ParseUint(portStr, 10, 32)
		if err != nil {
			t.Fatal("strconv.ParseUint: ", err)
		}
		ports = append(ports, port)
	}

	// Start a TLW server that resolves "dut001:22" to the real target addr/port.
	stopFunc, tlwAddr := faketlw.StartWiringServer(t, faketlw.WithDUTPortMap(map[faketlw.NamePort]faketlw.NamePort{
		{Name: targetName, Port: 22}:       {Name: hosts[0], Port: int32(ports[0])},
		{Name: companionDutName, Port: 22}: {Name: hosts[1], Port: int32(ports[1])},
	}), faketlw.WithCacheFileMap(map[string][]byte{"gs://a/b/c": []byte("abc")}),
		faketlw.WithDUTName(targetName))
	defer stopFunc()

	td.Cfg.Target = targetName
	td.Cfg.TLWServer = tlwAddr
	td.Cfg.Devservers = nil
	td.Cfg.CompanionDUTs = map[string]string{companionRole: companionDutName}
	testRunDownloadPrivateBundles(t, td)
}

func testRunDownloadPrivateBundles(t *gotesting.T, td *fakerunner.LocalTestData) {
	td.Cfg.RunLocal = true
	duts := make(map[string]struct{})
	for _, srv := range td.SrvData.Srvs {
		duts[srv.Addr().String()] = struct{}{}
	}
	dutsWithDownloadRequest := make(map[string]struct{})
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, nil)
		case jsonprotocol.RunnerDownloadPrivateBundlesMode:
			dutName := args.DownloadPrivateBundles.DUTName
			if strings.Contains(dutName, "@") {
				userHost := strings.SplitN(dutName, "@", 2)
				dutName = userHost[1]
			}
			if _, ok := duts[dutName]; !ok {
				t.Errorf("got unknown DUT name %v while downloading private bundle", dutName)
			}
			dutsWithDownloadRequest[dutName] = struct{}{}
			exp := jsonprotocol.RunnerDownloadPrivateBundlesArgs{
				Devservers:        td.Cfg.Devservers,
				DUTName:           args.DownloadPrivateBundles.DUTName,
				BuildArtifactsURL: td.Cfg.BuildArtifactsURL,
			}
			if diff := cmp.Diff(exp, *args.DownloadPrivateBundles,
				cmpopts.IgnoreFields(*args.DownloadPrivateBundles, "TLWServer")); diff != "" {
				t.Errorf("got args %+v; want %+v; diff=%v", *args.DownloadPrivateBundles, exp, diff)
			}
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerDownloadPrivateBundlesResult{})

			if td.Cfg.TLWServer != "" {
				// Try connecting to TLWServer through ssh port forwarding.
				if err := checkTLWServer(args.DownloadPrivateBundles.TLWServer); err != nil {
					t.Errorf("TLW server was not available: %v", err)
				}
			}
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.Cfg.DownloadPrivateBundles = true

	if _, err := Run(context.Background(), &td.Cfg, &td.State); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	if diff := cmp.Diff(dutsWithDownloadRequest, duts); diff != "" {
		t.Errorf("got DUTs with download request %+v; want %+v; diff=%v", dutsWithDownloadRequest, duts, diff)
	}
}

func TestRunTLW(t *gotesting.T) {
	const targetName = "the_dut"

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	host, portStr, err := net.SplitHostPort(td.Cfg.Target)
	if err != nil {
		t.Fatal("net.SplitHostPort: ", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatal("strconv.ParseUint: ", err)
	}

	// Start a TLW server that resolves "the_dut:22" to the real target addr/port.
	stopFunc, tlwAddr := faketlw.StartWiringServer(t, faketlw.WithDUTPortMap(map[faketlw.NamePort]faketlw.NamePort{
		{Name: targetName, Port: 22}: {Name: host, Port: int32(port)},
	}))
	defer stopFunc()

	td.Cfg.RunLocal = true
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, nil)
		}
		return 0
	}
	td.Cfg.Target = targetName
	td.Cfg.TLWServer = tlwAddr

	if _, err := Run(context.Background(), &td.Cfg, &td.State); err != nil {
		t.Errorf("Run failed: %v", err)
	}
}

// TestRunWithReports_LogStream tests Run() with fake Reports server and log stream.
func TestRunWithReports_LogStream(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t, 0)
	defer stopFunc()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()
	td.Cfg.ReportsServer = addr
	td.Cfg.RunLocal = true

	const (
		resultDir = "/tmp/tast/results/latest"
		test1Name = "foo.FirstTest"
		// Log file path for a test. Composed by handleTestStart() in results.go.
		test1Path    = "tests/foo.FirstTest/log.txt"
		test1Desc    = "First description"
		test1LogText = "Here's a test log message"
		test2Name    = "foo.SecondTest"
		test2Path    = "tests/foo.SecondTest/log.txt"
		test2Desc    = "Second description"
		test2LogText = "Here's another test log message"
	)
	td.Cfg.ResDir = resultDir
	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test_1",
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test_2",
			},
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			patterns := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(0, 0), NumTests: len(patterns)})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(10, 0), Info: jsonprotocol.EntityInfo{Name: test1Name}})
			mw.WriteMessage(&control.EntityLog{Time: time.Unix(15, 0), Name: test1Name, Text: test1LogText})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(20, 0), Name: test1Name})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(30, 0), Info: jsonprotocol.EntityInfo{Name: test2Name}})
			mw.WriteMessage(&control.EntityLog{Time: time.Unix(35, 0), Name: test2Name, Text: test2LogText})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(40, 0), Name: test2Name})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(50, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, tests)
		case jsonprotocol.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerListFixturesResult{})
		}
		return 0
	}
	if _, err := Run(context.Background(), &td.Cfg, &td.State); err != nil {
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

// TestRunWithReports_ReportResult tests Run() with fake Reports server and reporting results.
func TestRunWithReports_ReportResult(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t, 0)
	defer stopFunc()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()
	td.Cfg.ReportsServer = addr
	td.Cfg.RunLocal = true

	const (
		test1Name       = "foo.FirstTest"
		test1Desc       = "First description"
		test2Name       = "foo.SecondTest"
		test2Desc       = "Second description"
		test3Name       = "foo.ThirdTest"
		test3Desc       = "Third description"
		test3SkipReason = "Test skip reason"
	)
	test2Error := jsonprotocol.Error{
		Reason: "Intentionally failed",
		File:   "/tmp/file.go",
		Line:   21,
		Stack:  "None",
	}
	test2ErrorTime := time.Unix(35, 0)
	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: test1Name,
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: test2Name,
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: test3Name,
			},
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			patterns := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(0, 0), NumTests: len(patterns)})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(10, 0), Info: jsonprotocol.EntityInfo{Name: test1Name}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(20, 0), Name: test1Name})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(30, 0), Info: jsonprotocol.EntityInfo{Name: test2Name}})
			mw.WriteMessage(&control.EntityError{Time: test2ErrorTime, Name: test2Name, Error: test2Error})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(40, 0), Name: test2Name})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(45, 0), Info: jsonprotocol.EntityInfo{Name: test3Name}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(50, 0), Name: test3Name, SkipReasons: []string{test3SkipReason}})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(60, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, tests)
		case jsonprotocol.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerListFixturesResult{})
		}
		return 0
	}
	if _, err := Run(context.Background(), &td.Cfg, &td.State); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	test2ErrorTimeStamp, _ := ptypes.TimestampProto(test2ErrorTime)
	expectedResults := []*frameworkprotocol.ReportResultRequest{
		{
			Test: test1Name,
		},
		{
			Test: test2Name,
			Errors: []*frameworkprotocol.ErrorReport{
				{
					Time:   test2ErrorTimeStamp,
					Reason: test2Error.Reason,
					File:   test2Error.File,
					Line:   int32(test2Error.Line),
					Stack:  test2Error.Stack,
				},
			},
		},
		{
			Test:       test3Name,
			SkipReason: test3SkipReason,
		},
	}
	results := srv.Results()
	if diff := cmp.Diff(results, expectedResults); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}

}

// TestRunWithReports_ReportResultTerminate tests Run() stop testing after getting terminate
// response from reports server.
func TestRunWithReports_ReportResultTerminate(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t, 1)
	defer stopFunc()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()
	td.Cfg.ReportsServer = addr
	td.Cfg.RunLocal = true

	const (
		test1Name       = "foo.FirstTest"
		test1Desc       = "First description"
		test2Name       = "foo.SecondTest"
		test2Desc       = "Second description"
		test3Name       = "foo.ThirdTest"
		test3Desc       = "Third description"
		test3SkipReason = "Test skip reason"
	)
	test2Error := jsonprotocol.Error{
		Reason: "Intentionally failed",
		File:   "/tmp/file.go",
		Line:   21,
		Stack:  "None",
	}
	test2ErrorTime := time.Unix(35, 0)
	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: test1Name,
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: test2Name,
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: test3Name,
			},
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			patterns := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(0, 0), NumTests: len(patterns)})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(10, 0), Info: jsonprotocol.EntityInfo{Name: test1Name}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(20, 0), Name: test1Name})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(30, 0), Info: jsonprotocol.EntityInfo{Name: test2Name}})
			mw.WriteMessage(&control.EntityError{Time: test2ErrorTime, Name: test2Name, Error: test2Error})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(40, 0), Name: test2Name})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(45, 0), Info: jsonprotocol.EntityInfo{Name: test3Name}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(50, 0), Name: test3Name, SkipReasons: []string{test3SkipReason}})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(60, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, tests)
		case jsonprotocol.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(jsonprotocol.RunnerListFixturesResult{})
		}
		return 0
	}
	if _, err := Run(context.Background(), &td.Cfg, &td.State); err == nil {
		t.Error("Run unexpectedly succeeded despite termination request")
	}
	test2ErrorTimeStamp, _ := ptypes.TimestampProto(test2ErrorTime)
	expectedResults := []*frameworkprotocol.ReportResultRequest{
		{
			Test: test1Name,
		},
		{
			Test: test2Name,
			Errors: []*frameworkprotocol.ErrorReport{
				{
					Time:   test2ErrorTimeStamp,
					Reason: test2Error.Reason,
					File:   test2Error.File,
					Line:   int32(test2Error.Line),
					Stack:  test2Error.Stack,
				},
			},
		},
	}
	results := srv.Results()
	if diff := cmp.Diff(results, expectedResults); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}

}

// TestRunWithSkippedTests makes sure that tests with unsupported dependency
// would be skipped.
func TestRunWithSkippedTests(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.Cfg.RunLocal = true
	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Supported_1",
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Unsupported_1",
			},
			SkipReason: "Not Supported",
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Supported_2",
			},
		},
	}
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerRunTestsMode:
			patterns := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			var count int64 = 1
			mw.WriteMessage(&control.RunStart{Time: time.Unix(count, 0), NumTests: len(patterns)})
			for _, p := range patterns {
				count = count + 1
				mw.WriteMessage(&control.EntityStart{Time: time.Unix(count, 0), Info: jsonprotocol.EntityInfo{Name: p}})
				count = count + 1
				var skipReasons []string
				if strings.HasPrefix(p, "pkg.Unsupported") {
					skipReasons = append(skipReasons, "Not Supported")
				}
				mw.WriteMessage(&control.EntityEnd{Time: time.Unix(count, 0), Name: p, SkipReasons: skipReasons})
			}
			count = count + 1

			mw.WriteMessage(&control.RunEnd{Time: time.Unix(count, 0), OutDir: ""})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, tests)
		case jsonprotocol.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerListFixturesResult{})
		}
		return 0
	}
	results, err := Run(context.Background(), &td.Cfg, &td.State)
	if err != nil {
		t.Errorf("Run failed: %v", err)
	}
	if len(results) != len(tests) {
		t.Errorf("Got wrong number of results %v; want %v", len(results), len(tests))
	}
	for _, r := range results {
		if strings.HasPrefix(r.Name, "pkg.Supported") {
			if r.SkipReason != "" {
				t.Errorf("Test %q has SkipReason %q; want none", r.Name, r.SkipReason)
			}
		} else if r.SkipReason == "" {
			t.Errorf("Test %q has no SkipReason; want something", r.Name)

		}
	}
}

// TestListTests make sure list test can list all tests.
func TestListTests(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode:      jsonprotocol.RunnerListTestsMode,
			ListTests: &jsonprotocol.RunnerListTestsArgs{BundleGlob: fakerunner.MockLocalBundleGlob},
		})
		runner.WriteListTestsResultAsJSON(stdout, tests)
		return 0
	}
	td.Cfg.TotalShards = 1
	td.Cfg.RunLocal = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	results, err := listTests(context.Background(), &td.Cfg, &td.State, cc)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected := make([]*resultsjson.Result, len(tests))
	for i := 0; i < len(tests); i++ {
		expected[i] = &resultsjson.Result{Test: *resultsjson.NewTestFromEntityInfo(&tests[i].EntityInfo), SkipReason: tests[i].SkipReason}
	}
	if diff := cmp.Diff(results, expected); diff != "" {
		t.Errorf("Unexpected list of local tests (-got +want):\n%s", diff)
	}
}

// TestListTestsWithSharding make sure list test can list tests in specified shards.
func TestListTestsWithSharding(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode:      jsonprotocol.RunnerListTestsMode,
			ListTests: &jsonprotocol.RunnerListTestsArgs{BundleGlob: fakerunner.MockLocalBundleGlob},
		})

		runner.WriteListTestsResultAsJSON(stdout, tests)
		return 0
	}
	td.Cfg.TotalShards = 2
	td.Cfg.RunLocal = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	for i := 0; i < td.Cfg.TotalShards; i++ {
		td.Cfg.ShardIndex = i
		results, err := listTests(context.Background(), &td.Cfg, &td.State, cc)
		if err != nil {
			t.Error("Failed to list local tests: ", err)
		}
		expected := []*resultsjson.Result{
			{Test: *resultsjson.NewTestFromEntityInfo(&tests[i].EntityInfo), SkipReason: tests[i].SkipReason},
		}
		if diff := cmp.Diff(results, expected); diff != "" {
			t.Errorf("Unexpected list of local tests (-got +want):\n%s", diff)
		}
	}
}

// TestListTestsWithSkippedTests make sure list test can list skipped tests correctly.
func TestListTestsWithSkippedTests(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.SkippedTest",
				Desc: "Skipped test",
			},
			SkipReason: "Skip",
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode:      jsonprotocol.RunnerListTestsMode,
			ListTests: &jsonprotocol.RunnerListTestsArgs{BundleGlob: fakerunner.MockLocalBundleGlob},
		})
		runner.WriteListTestsResultAsJSON(stdout, tests)
		return 0
	}
	td.Cfg.TotalShards = 2
	td.Cfg.RunLocal = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	// Shard 0 should include all skipped tests.
	td.Cfg.ShardIndex = 0
	results, err := listTests(context.Background(), &td.Cfg, &td.State, cc)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected := []*resultsjson.Result{
		{Test: *resultsjson.NewTestFromEntityInfo(&tests[0].EntityInfo), SkipReason: tests[0].SkipReason},
		{Test: *resultsjson.NewTestFromEntityInfo(&tests[2].EntityInfo), SkipReason: tests[2].SkipReason},
	}
	if diff := cmp.Diff(results, expected); diff != "" {
		t.Errorf("Unexpected list of local tests in shard 0 (-got +want):\n%s", diff)
	}

	td.Cfg.ShardIndex = 1
	// Shard 1 should have only one test
	results, err = listTests(context.Background(), &td.Cfg, &td.State, cc)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected = []*resultsjson.Result{
		{Test: *resultsjson.NewTestFromEntityInfo(&tests[1].EntityInfo), SkipReason: tests[1].SkipReason},
	}
	if diff := cmp.Diff(results, expected); diff != "" {
		t.Errorf("Unexpected list of local tests in shard 1 (-got +want):\n%s", diff)
	}
}

// TestListTestsGetDUTInfo make sure GetDUTInfo is called when listTests is called.
func TestListTestsGetDUTInfo(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	called := false

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerGetDUTInfoMode:
			// Just check that GetDUTInfo is called; details of args are
			// tested in deps_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerGetDUTInfoResult{
				SoftwareFeatures: &protocol.SoftwareFeatures{
					Available: []string{"foo"}, // must report non-empty features
				},
			})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := listTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Error("listTests failed: ", err)
	}
	if !called {
		t.Error("runTests did not call getSoftwareFeatures")
	}
}

func TestRunTestsGetDUTInfo(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	called := false

	osVersion := "octopus-release/R86-13312.0.2020_07_02_1108"

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerGetDUTInfoMode:
			// Just check that GetDUTInfo is called; details of args are
			// tested in deps_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerGetDUTInfoResult{
				SoftwareFeatures: &protocol.SoftwareFeatures{
					Available: []string{"foo"}, // must report non-empty features
				},
				OSVersion: osVersion,
			})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := runTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Error("runTests failed: ", err)
	}

	expectedOSVersion := "Target version: " + osVersion
	if !strings.Contains(td.LogBuf.String(), expectedOSVersion) {
		t.Errorf("Cannot find %q in log buffer %v", expectedOSVersion, td.LogBuf.String())
	}
	if !called {
		t.Error("runTests did not call getSoftwareFeatures")
	}
}

func TestRunCollectSysInfo(t *gotesting.T) {
	ctx := context.Background()

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
	cfg := env.Config()
	state := env.State()

	if _, err := Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	if !called {
		t.Error("CollectSysInfo was not called")
	}
}

// TestRunTestsSkipTests check if runTests skipping testings correctly.
func TestRunTestsSkipTests(t *gotesting.T) {
	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name:         "unsupported.Test0",
				Desc:         "This is test 0",
				SoftwareDeps: []string{"has_dep"},
			},
			SkipReason: "dependency not available",
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{Name: "pkg.Test1", Desc: "This is test 1"},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{Name: "pkg.Test2", Desc: "This is test 2"},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{Name: "pkg.Test3", Desc: "This is test 3"},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{Name: "pkg.Test4", Desc: "This is test 4"},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name:         "unsupported.Test5",
				Desc:         "This is test 5",
				SoftwareDeps: []string{"has_dep"},
			},
			SkipReason: "dependency not available",
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{Name: "pkg.Test6", Desc: "This is test 6"},
		},
	}

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerGetDUTInfoMode:
			// Just check that GetDUTInfo is called; details of args are
			// tested in deps_test.go.
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerGetDUTInfoResult{
				SoftwareFeatures: &protocol.SoftwareFeatures{
					Available: []string{"a_feature"},
				},
			})
		case jsonprotocol.RunnerListTestsMode:
			runner.WriteListTestsResultAsJSON(stdout, tests)
		case jsonprotocol.RunnerRunTestsMode:
			testNames := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: len(testNames)})
			count := int64(2)
			for _, t := range testNames {
				mw.WriteMessage(&control.EntityStart{Time: time.Unix(count, 0), Info: jsonprotocol.EntityInfo{Name: t}})
				count++
				var skipReasons []string
				if strings.HasPrefix(t, "unsupported") {
					skipReasons = append(skipReasons, "dependency not available")
				}
				mw.WriteMessage(&control.EntityEnd{Time: time.Unix(count, 0), Name: t, SkipReasons: skipReasons})
				count++
			}
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(count, 0)})
		case jsonprotocol.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerListFixturesResult{})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	// List matching tests instead of running them.
	td.Cfg.LocalDataDir = "/tmp/data"
	td.Cfg.Patterns = []string{"*Test*"}
	td.Cfg.RunLocal = true
	td.Cfg.TotalShards = 2
	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	expectedPassed := 5
	expectedSkipped := len(tests) - 5
	passed := 0
	skipped := 0
	for shardIndex := 0; shardIndex < td.Cfg.TotalShards; shardIndex++ {
		td.State.DUTInfo = nil
		td.Cfg.ShardIndex = shardIndex
		testResults, err := runTests(context.Background(), &td.Cfg, &td.State, cc)
		if err != nil {
			t.Fatal("Failed to run tests: ", err)
		}
		for _, t := range testResults {
			if t.SkipReason == "" {
				passed++
			} else {
				skipped++
			}
		}
	}
	if passed != expectedPassed {
		t.Errorf("runTests returned %d passed tests; want %d", passed, expectedPassed)
	}
	if skipped != expectedSkipped {
		t.Errorf("runTests returned %d skipped tests; want %d", skipped, expectedSkipped)
	}
}
