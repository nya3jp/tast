// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/subcommands"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/fakereports"
	"chromiumos/tast/internal/faketlw"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
)

func TestRunPartialRun(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Set a nonexistent path for the remote runner so that it will fail.
	td.cfg.runLocal = true
	td.cfg.runRemote = true
	const testName = "pkg.Test"
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""})
		case runner.ListTestsMode:
			tests := []testing.EntityWithRunnabilityInfo{
				{
					EntityInfo: testing.EntityInfo{
						Name: testName,
					},
				},
			}
			json.NewEncoder(stdout).Encode(tests)
		}
		return 0
	}
	td.cfg.remoteRunner = filepath.Join(td.tempDir, "missing_remote_test_runner")

	status, _ := Run(context.Background(), &td.cfg)
	if status.ExitCode != subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitFailure, td.logbuf.String())
	}
}

func TestRunError(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.runLocal = true
	td.cfg.KeyFile = "" // force SSH auth error

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v", status, subcommands.ExitFailure)
	} else if !status.FailedBeforeRun {
		// local()'s initial connection attempt will fail, so we won't try to run tests.
		t.Error("Run() incorrectly reported that failure did not occur before trying to run tests")
	}
}

func TestRunEphemeralDevserver(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.runLocal = true
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.ListTestsMode:
			json.NewEncoder(stdout).Encode([]testing.EntityWithRunnabilityInfo{})
		}
		return 0
	}
	td.cfg.devservers = nil // clear the default mock devservers set in newLocalTestData
	td.cfg.useEphemeralDevserver = true

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}

	exp := []string{fmt.Sprintf("http://127.0.0.1:%d", ephemeralDevserverPort)}
	if !reflect.DeepEqual(td.cfg.devservers, exp) {
		t.Errorf("Run() set devserver=%v; want %v", td.cfg.devservers, exp)
	}
}

func TestRunDownloadPrivateBundles(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()
	td.cfg.devservers = []string{"http://example.com:8080"}
	testRunDownloadPrivateBundles(t, td)
}

func TestRunDownloadPrivateBundlesWithTLW(t *gotesting.T) {
	const targetName = "dut001"
	td := newLocalTestData(t)
	defer td.close()

	host, portStr, err := net.SplitHostPort(td.cfg.Target)
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

	td.cfg.Target = targetName
	td.cfg.tlwServer = tlwAddr
	td.cfg.devservers = nil
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

func testRunDownloadPrivateBundles(t *gotesting.T, td *localTestData) {
	td.cfg.runLocal = true
	called := false
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.ListTestsMode:
			json.NewEncoder(stdout).Encode([]testing.EntityWithRunnabilityInfo{})
		case runner.DownloadPrivateBundlesMode:
			exp := runner.DownloadPrivateBundlesArgs{
				Devservers:        td.cfg.devservers,
				DUTName:           td.cfg.Target,
				BuildArtifactsURL: td.cfg.buildArtifactsURL,
			}
			if diff := cmp.Diff(exp, *args.DownloadPrivateBundles,
				cmpopts.IgnoreFields(*args.DownloadPrivateBundles, "TLWServer")); diff != "" {
				t.Errorf("got args %+v; want %+v; diff=%v", *args.DownloadPrivateBundles, exp, diff)
			}
			called = true
			json.NewEncoder(stdout).Encode(&runner.DownloadPrivateBundlesResult{})

			if td.cfg.tlwServer != "" {
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

	td.cfg.downloadPrivateBundles = true

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if !called {
		t.Errorf("Run did not call downloadPrivateBundles")
	}
}

func TestRunTLW(t *gotesting.T) {
	const targetName = "the_dut"

	td := newLocalTestData(t)
	defer td.close()

	host, portStr, err := net.SplitHostPort(td.cfg.Target)
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

	td.cfg.runLocal = true
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.ListTestsMode:
			json.NewEncoder(stdout).Encode([]testing.EntityWithRunnabilityInfo{})
		}
		return 0
	}
	td.cfg.Target = targetName
	td.cfg.tlwServer = tlwAddr

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
}

// TestRunWithReports tests Run() with fake Reports server.
func TestRunWithReports_LogStream(t *gotesting.T) {
	srv, stopFunc, addr := fakereports.Start(t)
	defer stopFunc()

	td := newLocalTestData(t)
	defer td.close()
	td.cfg.reportsServer = addr
	td.cfg.runLocal = true

	const (
		test1Name    = "foo.FirstTest"
		test1Desc    = "First description"
		test1LogText = "Here's a test log message"
		test2Name    = "foo.SecondTest"
		test2Desc    = "Second description"
		test2LogText = "Here's another test log message"
	)
	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test_1",
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test_2",
			},
		},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			patterns := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(0, 0), NumTests: len(patterns)})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(10, 0), Info: testing.EntityInfo{Name: test1Name}})
			mw.WriteMessage(&control.EntityLog{Time: time.Unix(15, 0), Name: test1Name, Text: test1LogText})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(20, 0), Name: test1Name})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(30, 0), Info: testing.EntityInfo{Name: test2Name}})
			mw.WriteMessage(&control.EntityLog{Time: time.Unix(35, 0), Name: test2Name, Text: test2LogText})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(40, 0), Name: test2Name})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(50, 0), OutDir: ""})
		case runner.ListTestsMode:
			json.NewEncoder(stdout).Encode(tests)
		}
		return 0
	}
	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if str := string(srv.GetLog(test1Name)); !strings.Contains(str, test1LogText) {
		t.Errorf("Expected log not received for test 1; got %q; should contain %q", str, test1LogText)
	}
	if str := string(srv.GetLog(test2Name)); !strings.Contains(str, test2LogText) {
		t.Errorf("Expected log not received for test 2; got %q; should contain %q", str, test2LogText)
	}
	if str := string(srv.GetLog(test1Name)); strings.Contains(str, test2LogText) {
		t.Errorf("Unexpected log found in test 1 log; got %q; should not contain %q", str, test2LogText)
	}
	if str := string(srv.GetLog(test2Name)); strings.Contains(str, test1LogText) {
		t.Errorf("Unexpected log found in test 2 log; got %q; should not contain %q", str, test1LogText)
	}
}

// TestRunWithSkippedTests makes sure that tests with unsupported dependency
// would be skipped.
func TestRunWithSkippedTests(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.runLocal = true
	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Supported_1",
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Unsupported_1",
			},
			SkipReason: "Not Supported",
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Supported_2",
			},
		},
	}
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			patterns := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			var count int64 = 1
			mw.WriteMessage(&control.RunStart{Time: time.Unix(count, 0), NumTests: len(patterns)})
			for _, p := range patterns {
				count = count + 1
				mw.WriteMessage(&control.EntityStart{Time: time.Unix(count, 0), Info: testing.EntityInfo{Name: p}})
				count = count + 1
				var skipReasons []string
				if strings.HasPrefix(p, "pkg.Unsupported") {
					skipReasons = append(skipReasons, "Not Supported")
				}
				mw.WriteMessage(&control.EntityEnd{Time: time.Unix(count, 0), Name: p, SkipReasons: skipReasons})
			}
			count = count + 1

			mw.WriteMessage(&control.RunEnd{Time: time.Unix(count, 0), OutDir: ""})
		case runner.ListTestsMode:

			json.NewEncoder(stdout).Encode(tests)
		}
		return 0
	}
	status, results := Run(context.Background(), &td.cfg)
	if status.ExitCode == subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
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

// TODO(crbug.com/982171): Add a test that runs remote tests successfully.
// This may require merging LocalTestData and RemoteTestData into one.
