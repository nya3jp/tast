// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	frameworkprotocol "chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/devserver/devservertest"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/fakereports"
	"chromiumos/tast/internal/run/reporting"
	"chromiumos/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/testing"
)

// resultsCmpOpts is a common options used to compare []*resultsjson.Result.
var resultsCmpOpts = []cmp.Option{
	cmpopts.IgnoreFields(resultsjson.Result{}, "Start", "End"),
	cmpopts.IgnoreFields(resultsjson.Test{}, "Timeout"),
	cmpopts.IgnoreFields(resultsjson.Error{}, "Time", "File", "Line", "Stack"),
}

func unmarshalStreamedResults(b []byte) ([]*resultsjson.Result, error) {
	decoder := json.NewDecoder(bytes.NewBuffer(b))
	var results []*resultsjson.Result
	for decoder.More() {
		var r resultsjson.Result
		if err := decoder.Decode(&r); err != nil {
			return nil, err
		}
		results = append(results, &r)
	}
	return results, nil
}

func TestRun(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.ResDir(), reporting.LegacyResultsFilename)); err != nil {
		t.Errorf("Results were not saved: %v", err)
	}
}

func TestRunNoTestToRun(t *gotesting.T) {
	// No test in bundles.
	env := runtest.SetUp(t, runtest.WithLocalBundles(testing.NewRegistry("bundle")), runtest.WithRemoteBundles(testing.NewRegistry("bundle")))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Patterns = []string{"(foobar)"} // an attribute expression matching no test
	})
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	// Results are not written in the case no test was run.
	if _, err := os.Stat(filepath.Join(cfg.ResDir(), reporting.LegacyResultsFilename)); err == nil {
		t.Error("Results were saved despite there was no test to run")
	} else if !os.IsNotExist(err) {
		t.Errorf("Failed to check if results were saved: %v", err)
	}
}

func TestRunPartialRun(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		// Set a nonexistent path for the remote runner so that it will fail.
		cfg.RemoteRunner = filepath.Join(env.TempDir(), "missing_remote_test_runner")
	})
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite missing remote_test_runner")
	}
}

func TestRunError(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.KeyFile = "" // force SSH auth error
	})
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err == nil {
		t.Error("Run unexpectedly succeeded despite unaccessible SSH server")
	}
}

func TestRunResults(t *gotesting.T) {
	localPass := &testing.TestInstance{
		Name:    "local.Pass",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localFail := &testing.TestInstance{
		Name:    "local.Fail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failed")
		},
	}
	localSkip := &testing.TestInstance{
		Name:         "local.Skip",
		Timeout:      time.Minute,
		SoftwareDeps: []string{"missing"},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localPass)
	localReg.AddTestInstance(localFail)
	localReg.AddTestInstance(localSkip)

	remotePass := &testing.TestInstance{
		Name:    "remote.Pass",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	remoteFail := &testing.TestInstance{
		Name:    "remote.Fail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failed")
		},
	}
	remoteSkip := &testing.TestInstance{
		Name:         "remote.Skip",
		Timeout:      time.Minute,
		SoftwareDeps: []string{"missing"},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remotePass)
	remoteReg.AddTestInstance(remoteFail)
	remoteReg.AddTestInstance(remoteSkip)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Unavailable: []string{"missing"},
						},
					},
				},
			}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	results, err := run.Run(ctx, cfg, state)
	if err != nil {
		t.Errorf("Run failed: %v", err)
	}

	expected := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:         "local.Skip",
				SoftwareDeps: []string{"missing"},
				Timeout:      time.Minute,
				Bundle:       "bundle",
			},
			OutDir:     filepath.Join(cfg.ResDir(), "tests/local.Skip"),
			SkipReason: "missing SoftwareDeps: missing",
		},
		{
			Test: resultsjson.Test{
				Name:    "local.Fail",
				Timeout: time.Minute,
				Bundle:  "bundle",
			},
			OutDir: filepath.Join(cfg.ResDir(), "tests/local.Fail"),
			Errors: []resultsjson.Error{{Reason: "Failed"}},
		},
		{
			Test: resultsjson.Test{
				Name:    "local.Pass",
				Timeout: time.Minute,
				Bundle:  "bundle",
			},
			OutDir: filepath.Join(cfg.ResDir(), "tests/local.Pass"),
		},
		{
			Test: resultsjson.Test{
				Name:         "remote.Skip",
				SoftwareDeps: []string{"missing"},
				Timeout:      time.Minute,
				Bundle:       "bundle",
			},
			SkipReason: "missing SoftwareDeps: missing",
			OutDir:     filepath.Join(cfg.ResDir(), "tests/remote.Skip"),
		},
		{
			Test: resultsjson.Test{
				Name:    "remote.Fail",
				Timeout: time.Minute,
				Bundle:  "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failed"}},
			OutDir: filepath.Join(cfg.ResDir(), "tests/remote.Fail"),
		},
		{
			Test: resultsjson.Test{
				Name:    "remote.Pass",
				Timeout: time.Minute,
				Bundle:  "bundle",
			},
			OutDir: filepath.Join(cfg.ResDir(), "tests/remote.Pass"),
		},
	}

	// Results returned from the function call.
	if diff := cmp.Diff(results, expected, resultsCmpOpts...); diff != "" {
		t.Errorf("Returned results mismatch (-got +want):\n%s", diff)
	}

	// Results in results.json.
	if b, err := ioutil.ReadFile(filepath.Join(cfg.ResDir(), reporting.LegacyResultsFilename)); err != nil {
		t.Errorf("Failed to read %s: %v", reporting.LegacyResultsFilename, err)
	} else if err := json.Unmarshal(b, &results); err != nil {
		t.Errorf("Failed to parse %s: %v", reporting.LegacyResultsFilename, err)
	} else if diff := cmp.Diff(results, expected, resultsCmpOpts...); diff != "" {
		t.Errorf("%s mismatch (-got +want):\n%s", reporting.LegacyResultsFilename, diff)
	}

	// Results in streamed_results.json.
	if b, err := ioutil.ReadFile(filepath.Join(cfg.ResDir(), reporting.StreamedResultsFilename)); err != nil {
		t.Errorf("Failed to read %s: %v", reporting.StreamedResultsFilename, err)
	} else if results, err := unmarshalStreamedResults(b); err != nil {
		t.Errorf("Failed to parse %s: %v", reporting.StreamedResultsFilename, err)
	} else if diff := cmp.Diff(results, expected, resultsCmpOpts...); diff != "" {
		t.Errorf("%s mismatch (-got +want):\n%s", reporting.StreamedResultsFilename, diff)
	}
}

func TestRunLogs(t *gotesting.T) {
	localPass := &testing.TestInstance{
		Name:    "local.Pass",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Hello from local.Pass")
		},
	}
	localFail := &testing.TestInstance{
		Name:    "local.Fail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Oops from local.Fail")
		},
	}
	localSkip := &testing.TestInstance{
		Name:         "local.Skip",
		Timeout:      time.Minute,
		SoftwareDeps: []string{"missing"},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localPass)
	localReg.AddTestInstance(localFail)
	localReg.AddTestInstance(localSkip)

	remotePass := &testing.TestInstance{
		Name:    "remote.Pass",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Hello from remote.Pass")
		},
	}
	remoteFail := &testing.TestInstance{
		Name:    "remote.Fail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Oops from remote.Fail")
		},
	}
	remoteSkip := &testing.TestInstance{
		Name:         "remote.Skip",
		Timeout:      time.Minute,
		SoftwareDeps: []string{"missing"},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remotePass)
	remoteReg.AddTestInstance(remoteFail)
	remoteReg.AddTestInstance(remoteSkip)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Unavailable: []string{"missing"},
						},
					},
				},
			}, nil
		}),
	)
	logger := loggingtest.NewLogger(t, logging.LevelInfo) // drop debug messages
	ctx := logging.AttachLoggerNoPropagation(env.Context(), logger)
	cfg := env.Config(nil)
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	// Inspect per-test log files.
	for _, tc := range []struct {
		relPath string
		want    string
	}{
		// local.Skip
		{"tests/local.Skip/log.txt", "Started test local.Skip"},
		{"tests/local.Skip/log.txt", "Skipped test local.Skip due to missing dependencies: missing SoftwareDeps: missing"},
		// local.Fail
		{"tests/local.Fail/log.txt", "Started test local.Fail"},
		{"tests/local.Fail/log.txt", "Oops from local.Fail"},
		{"tests/local.Fail/log.txt", "Completed test local.Fail"},
		// local.Pass
		{"tests/local.Pass/log.txt", "Started test local.Pass"},
		{"tests/local.Pass/log.txt", "Hello from local.Pass"},
		{"tests/local.Pass/log.txt", "Completed test local.Pass"},
		// remote.Skip
		{"tests/remote.Skip/log.txt", "Started test remote.Skip"},
		{"tests/remote.Skip/log.txt", "Skipped test remote.Skip due to missing dependencies: missing SoftwareDeps: missing"},
		// remote.Fail
		{"tests/remote.Fail/log.txt", "Started test remote.Fail"},
		{"tests/remote.Fail/log.txt", "Oops from remote.Fail"},
		{"tests/remote.Fail/log.txt", "Completed test remote.Fail"},
		// remote.Pass
		{"tests/remote.Pass/log.txt", "Started test remote.Pass"},
		{"tests/remote.Pass/log.txt", "Hello from remote.Pass"},
		{"tests/remote.Pass/log.txt", "Completed test remote.Pass"},
	} {
		b, err := ioutil.ReadFile(filepath.Join(cfg.ResDir(), tc.relPath))
		if err != nil {
			t.Errorf("Failed to read %s: %v", tc.relPath, err)
			continue
		}
		got := string(b)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s doesn't contain an expected string: got %q, want %q", tc.relPath, got, tc.want)
		}
	}

	// Inspect full logs.
	fullLogs := logger.String()
	for _, msg := range []string{
		// local.Skip
		"Started test local.Skip",
		"Skipped test local.Skip due to missing dependencies: missing SoftwareDeps: missing",
		// local.Pass
		"Started test local.Pass",
		"Hello from local.Pass",
		"Completed test local.Pass",
		// local.Fail
		"Started test local.Fail",
		"Oops from local.Fail",
		"Completed test local.Fail",
		// remote.Skip
		"Started test remote.Skip",
		"Skipped test remote.Skip due to missing dependencies: missing SoftwareDeps: missing",
		// remote.Pass
		"Started test remote.Pass",
		"Hello from remote.Pass",
		"Completed test remote.Pass",
		// remote.Fail
		"Started test remote.Fail",
		"Oops from remote.Fail",
		"Completed test remote.Fail",
	} {
		if !strings.Contains(fullLogs, msg) {
			// Note: Full logs have been already sent to unit test logs by
			// loggingtest.Logger, so we don't print fullLogs here.
			t.Errorf("Full logs doesn't contain %q", msg)
		}
	}
}

func TestRunOutputFiles(t *gotesting.T) {
	localOut := &testing.TestInstance{
		Name:    "local.Out",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "out.txt"), []byte("Hello from local.Out"), 0644); err != nil {
				t.Errorf("local.Out: failed to write out.txt: %v", err)
			}
		},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localOut)

	remoteOut := &testing.TestInstance{
		Name:    "remote.Out",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "out.txt"), []byte("Hello from remote.Out"), 0644); err != nil {
				t.Errorf("remote.Out: failed to write out.txt: %v", err)
			}
		},
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remoteOut)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
	)
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{"tests/local.Out/out.txt", "Hello from local.Out"},
		{"tests/remote.Out/out.txt", "Hello from remote.Out"},
	} {
		b, err := ioutil.ReadFile(filepath.Join(cfg.ResDir(), tc.relPath))
		if err != nil {
			t.Errorf("Failed to read %s: %v", tc.relPath, err)
			continue
		}
		got := string(b)
		if got != tc.want {
			t.Errorf("%s mismatch: got %q, want %q", tc.relPath, got, tc.want)
		}
	}
}

func TestRunEphemeralDevserver(t *gotesting.T) {
	env := runtest.SetUp(t, runtest.WithOnRunLocalTestsInit(func(init *protocol.RunTestsInit, _ *protocol.BundleConfig) {
		if ds := init.GetRunConfig().GetServiceConfig().GetDevservers(); len(ds) != 1 {
			t.Errorf("Local runner: devservers=%#v; want 1 entry", ds)
		}
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.UseEphemeralDevserver = true
	})
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
			if diff := cmp.Diff(req.GetServiceConfig(), want, protocmp.Transform()); diff != "" {
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
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Devservers = []string{ds.URL}
		cfg.DownloadPrivateBundles = true
	})
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	wantCalled := map[string]struct{}{"dut0": {}, "dut1": {}, "dut2": {}}
	if diff := cmp.Diff(called, wantCalled); diff != "" {
		t.Errorf("DownloadPrivateBundles not called (-got +want):\n%s", diff)
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
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.ReportsServer = addr
	})
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
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.ReportsServer = addr
	})
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
	// make sure StartTime and Duration are not nil.
	for _, r := range results {
		if r.StartTime == nil || r.Duration == nil {
			t.Errorf("Test result for %s return nil on start time or duration", r.Test)
		}
	}
	cmpOptIgnoreErrFields := cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time", "File", "Line", "Stack")
	cmpOptIgnoreTimeFields := cmpopts.IgnoreFields(frameworkprotocol.ReportResultRequest{}, "StartTime", "Duration")
	cmpOptIgnoreUnexported := cmpopts.IgnoreUnexported(frameworkprotocol.ReportResultRequest{})
	cmpOptIgnoreUnexportedErr := cmpopts.IgnoreUnexported(frameworkprotocol.ErrorReport{})
	if diff := cmp.Diff(results, expectedResults,
		cmpOptIgnoreTimeFields, cmpOptIgnoreErrFields,
		cmpOptIgnoreUnexported, cmpOptIgnoreUnexportedErr); diff != "" {
		t.Errorf("Got unexpected results (-got +want):\n%s", diff)
	}
	// Check if result has start time and duration.
	for i, r := range results {
		if r.StartTime == nil || r.Duration == nil {
			t.Errorf("Test result for %s should have start time and duration", expectedResults[i].Test)
		}
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
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.ReportsServer = addr
	})
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
	cmpOptIgnoreErrFields := cmpopts.IgnoreFields(frameworkprotocol.ErrorReport{}, "Time", "File", "Line", "Stack")
	cmpOptIgnoreTimeFields := cmpopts.IgnoreFields(frameworkprotocol.ReportResultRequest{}, "StartTime", "Duration")
	cmpOptIgnoreUnexported := cmpopts.IgnoreUnexported(frameworkprotocol.ReportResultRequest{})
	cmpOptIgnoreUnexportedErr := cmpopts.IgnoreUnexported(frameworkprotocol.ErrorReport{})
	if diff := cmp.Diff(results, expectedResults,
		cmpOptIgnoreTimeFields, cmpOptIgnoreErrFields,
		cmpOptIgnoreUnexported, cmpOptIgnoreUnexportedErr); diff != "" {
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
	cfg := env.Config(nil)
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
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Mode = config.ListTestsMode
	})
	state := env.State()

	results, err := run.Run(ctx, cfg, state)
	if err != nil {
		t.Errorf("Run failed: %v", err)
	}

	localTestMeta, _ := resultsjson.NewTest(localTest.EntityProto())
	remoteTestMeta, _ := resultsjson.NewTest(remoteTest.EntityProto())
	skippedTestMeta, _ := resultsjson.NewTest(skippedTest.EntityProto())
	expected := []*resultsjson.Result{
		{Test: *localTestMeta},
		{Test: *remoteTestMeta},
		{Test: *skippedTestMeta, SkipReason: "missing SoftwareDeps: missing"},
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
	state := env.State()

	localTestMeta, _ := resultsjson.NewTest(localTest.EntityProto())
	remoteTestMeta, _ := resultsjson.NewTest(remoteTest.EntityProto())
	skippedTestMeta, _ := resultsjson.NewTest(skippedTest.EntityProto())

	for shardIndex, expected := range [][]*resultsjson.Result{
		{
			{Test: *localTestMeta},
			{Test: *remoteTestMeta},
		},
		{
			{Test: *skippedTestMeta, SkipReason: "missing SoftwareDeps: missing"},
		},
	} {
		t.Run(fmt.Sprintf("shard%d", shardIndex), func(t *gotesting.T) {
			cfg := env.Config(func(cfg *config.MutableConfig) {
				cfg.Mode = config.ListTestsMode
				cfg.TotalShards = 2
				cfg.ShardIndex = shardIndex
			})

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

func TestRunDumpDUTInfo(t *gotesting.T) {
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
	cfg := env.Config(nil)
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	const expectedOSVersion = "Target version: " + osVersion
	if logs := logger.String(); !strings.Contains(logs, expectedOSVersion) {
		t.Errorf("Cannot find %q in log buffer %q", expectedOSVersion, logs)
	}

	// Make sure dut-info.txt is created.
	if _, err := os.Stat(filepath.Join(cfg.ResDir(), run.DUTInfoFile)); err != nil {
		t.Errorf("Failed to stat %s: %v", run.DUTInfoFile, err)
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
			if diff := cmp.Diff(req.GetInitialState(), initState, protocmp.Transform()); diff != "" {
				t.Errorf("SysInfoState mismatch (-got +want):\n%s", diff)
			}
			return &protocol.CollectSysInfoResponse{}, nil
		}))
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	if !called {
		t.Error("CollectSysInfo was not called")
	}
}

// TestRunWithGlobalRuntimeVars tests run time variables are correctly pass on to the tests.
func TestRunWithGlobalRuntimeVars(t *gotesting.T) {
	localReg := testing.NewRegistry("bundle")
	remoteReg := testing.NewRegistry("bundle")

	var1 := testing.NewVarString("var1", "", "description")
	localReg.AddVar(var1)
	var2 := testing.NewVarString("var2", "", "description")
	localReg.AddVar(var2)
	var3 := testing.NewVarString("var3", "", "description")
	remoteReg.AddVar(var3)
	var4 := testing.NewVarString("var4", "", "description")
	remoteReg.AddVar(var4)

	localTest := &testing.TestInstance{
		Name: "localTest",
		Func: func(ctx context.Context, s *testing.State) {},
	}
	remoteTest := &testing.TestInstance{
		Name: "remoteTest",
		Func: func(ctx context.Context, s *testing.State) {},
	}

	localReg.AddTestInstance(localTest)
	remoteReg.AddTestInstance(remoteTest)

	env := runtest.SetUp(t, runtest.WithLocalBundles(localReg), runtest.WithRemoteBundles(remoteReg))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.TestVars = map[string]string{
			"var1": "value1",
			"var3": "value3",
		}
	})
	state := env.State()

	// The variable var1 and var3 will have non-default values.

	if _, err := run.Run(ctx, cfg, state); err != nil {
		t.Errorf("Run failed: %v", err)
	}
	vars := []*testing.VarString{var1, var2, var3, var4}
	for _, v := range vars {
		if v.Value() != cfg.TestVars()[v.Name()] {
			t.Errorf("Run set global runtime variable %q to %q; want %q", v.Name(), v.Value(), cfg.TestVars()[v.Name()])
		}
	}
}

// TestRunWithVerifyTestNameFail tests that an err is raised when non-existent test is given during a list.
func TestRunWithVerifyTestNameFail(t *gotesting.T) {
	const (
		bundleName    = "bundle"
		localTestName = "pkg.LocalTest"
	)

	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    localTestName,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			t.Errorf("%s was run despite bad name", localTestName)
		},
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Patterns = []string{"pkg.LocalTest", "pkg.NonExistingTest"}
	})
	state := env.State()

	_, err := run.Run(ctx, cfg, state)
	if err == nil {
		t.Errorf("Run did not err on missing pattern")
	}
}

// TestRunWithVerifyTestPatternRuns tests that when a pattern is provided the test will not err on test matching.
func TestRunWithVerifyTestPatternRuns(t *gotesting.T) {
	const (
		bundleName    = "bundle"
		localTestName = "pkg.LocalTest"
	)

	localReg := testing.NewRegistry(bundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    localTestName,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			t.Errorf("%s was run despite bad name", localTestName)
		},
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Patterns = []string{`("name:pkg.LocalTest" || "name:pkg.NonExistingTest")`}
	})
	state := env.State()

	_, err := run.Run(ctx, cfg, state)
	if err != nil {
		t.Errorf("Unexpected err during test patten search: %s", err)
	}
}
