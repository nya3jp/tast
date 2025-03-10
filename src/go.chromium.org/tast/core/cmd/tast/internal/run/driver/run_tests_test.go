// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/cmd/tast/internal/run/runtest"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/reporting"
	"go.chromium.org/tast/core/internal/run/resultsjson"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/internal/testing/testfixture"
	"go.chromium.org/tast/core/internal/usercode"
	"go.chromium.org/tast/core/testutil"
)

// resultsCmpOpts is a common options used to compare []resultsjson.Result.
var resultsCmpOpts = []cmp.Option{
	cmpopts.IgnoreFields(resultsjson.Result{}, "Start", "End", "OutDir"),
	cmpopts.IgnoreFields(resultsjson.Test{}, "Timeout"),
	cmpopts.IgnoreFields(resultsjson.Error{}, "Time", "File", "Line", "Stack"),
}

func TestDriver_RunTests(t *gotesting.T) {
	bundle1Local := testing.NewRegistry("bundle1")
	bundle1Local.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local1",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	bundle2Local := testing.NewRegistry("bundle2")
	bundle2Local.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local2",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failed")
		},
	})
	bundle3Local := testing.NewRegistry("bundle3")
	bundle3Local.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local3",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
		SoftwareDeps: map[string][]string{
			"":     []string{},
			"dut1": []string{"mock"}},
	})
	bundle1Remote := testing.NewRegistry("bundle1")
	bundle1Remote.AddTestInstance(&testing.TestInstance{
		Name:    "test.Remote1",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	bundle2Remote := testing.NewRegistry("bundle2")
	bundle2Remote.AddTestInstance(&testing.TestInstance{
		Name:    "test.Remote2",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failed")
		},
	})
	bundle3Remote := testing.NewRegistry("bundle3")
	bundle3Remote.AddTestInstance(&testing.TestInstance{
		Name:    "test.Remote3",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
		SoftwareDeps: map[string][]string{
			"":     []string{},
			"dut1": []string{"mock"}},
	})
	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundle1Local, bundle2Local, bundle3Local),
		runtest.WithRemoteBundles(bundle1Remote, bundle2Remote, bundle3Remote),
		runtest.WithCompanionDUT("dut1"),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	if err != nil {
		t.Errorf("RunTests failed: %v", err)
	}

	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.Local1",
				Bundle: "bundle1",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote1",
				Bundle: "bundle1",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local2",
				Bundle: "bundle2",
			},
			Errors: []resultsjson.Error{{
				Reason: "Failed",
			}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote2",
				Bundle: "bundle2",
			},
			Errors: []resultsjson.Error{{
				Reason: "Failed",
			}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local3",
				Bundle: "bundle3",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote3",
				Bundle: "bundle3",
			},
		},
	}
	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}
}

func TestDriver_RunTests_RemoteFixture(t *gotesting.T) {
	fixtureActive := false

	bundle1Local := testing.NewRegistry("bundle1")
	bundle1Local.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local1",
		Timeout: time.Minute,
		Fixture: "fixture.Remote1",
		Func: func(ctx context.Context, s *testing.State) {
			if !fixtureActive {
				t.Error("test.Local1 was run without setting up fixture.Remote1")
			}
		},
	})
	bundle2Local := testing.NewRegistry("bundle2")
	bundle2Local.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local2",
		Timeout: time.Minute,
		Fixture: "fixture.Remote2",
		Func: func(ctx context.Context, s *testing.State) {
			if !driver.ShouldRunTestsRecursively() {
				t.Error("test.Local2 was run unexpectedly")
			}
		},
	})
	bundle1Remote := testing.NewRegistry("bundle1")
	bundle1Remote.AddFixtureInstance(&testing.FixtureInstance{
		Name: "fixture.Remote1",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				fixtureActive = true
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				fixtureActive = false
			}),
		),
	})
	bundle2Remote := testing.NewRegistry("bundle2")
	bundle2Remote.AddFixtureInstance(&testing.FixtureInstance{
		Name: "fixture.Remote2",
		Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			if !driver.ShouldRunTestsRecursively() {
				t.Error("fixture.Remote2 was set up unexpectedly")
			}
			return nil
		})),
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundle1Local, bundle2Local),
		runtest.WithRemoteBundles(bundle1Remote, bundle2Remote),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		// Set the primary bundle.
		cfg.PrimaryBundle = "bundle1"
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	if err != nil {
		t.Errorf("RunTests failed: %v", err)
	}

	wantFixtureError := []resultsjson.Error{{
		Reason: "Local-remote fixture dependencies are not yet supported in non-primary bundles (b/187957164)",
	}}
	if driver.ShouldRunTestsRecursively() {
		wantFixtureError = nil
	}

	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:    "test.Local1",
				Bundle:  "bundle1",
				Fixture: "fixture.Remote1",
			},
		},
		{
			Test: resultsjson.Test{
				Name:    "test.Local2",
				Bundle:  "bundle2",
				Fixture: "fixture.Remote2",
			},
			Errors: wantFixtureError,
		},
	}
	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}
}

func TestDriver_RunTests_RetryTests(t *gotesting.T) {
	bundleLocal := testing.NewRegistry("bundle")
	for _, name := range []string{"test.Local1", "test.Local2", "test.Local3"} {
		bundleLocal.AddTestInstance(&testing.TestInstance{
			Name:    name,
			Timeout: time.Minute,
			Func: func(ctx context.Context, s *testing.State) {
				// Simulate a test bundle crash.
				usercode.ForceErrorForTesting(errors.New("intentional crash"))
			},
		})
	}
	bundleRemote := testing.NewRegistry("bundle")

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundleLocal),
		runtest.WithRemoteBundles(bundleRemote),
	)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	if err != nil {
		t.Errorf("RunTests failed: %v", err)
	}

	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.Local1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "intentional crash (see log for goroutine dump)"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local2",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "intentional crash (see log for goroutine dump)"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local3",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "intentional crash (see log for goroutine dump)"}},
		},
	}
	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}
}

func TestDriver_RunTests_MaxTestFailures(t *gotesting.T) {
	bundleLocal := testing.NewRegistry("bundle")
	for _, name := range []string{"test.Local1", "test.Local2", "test.Local3"} {
		bundleLocal.AddTestInstance(&testing.TestInstance{
			Name:    name,
			Timeout: time.Minute,
			Func: func(ctx context.Context, s *testing.State) {
				s.Error("Failure")
			},
		})
	}
	bundleRemote := testing.NewRegistry("bundle")

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundleLocal),
		runtest.WithRemoteBundles(bundleRemote),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.MaxTestFailures = 2
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	if err == nil {
		t.Error("RunTests unexpectedly succeeded")
	}

	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.Local1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local2",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		// Third test is missing.
	}
	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}
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

func TestDriver_RunTests_WithRepeats(t *gotesting.T) {
	bundleLocal := testing.NewRegistry("bundle")
	bundleLocal.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
		},
	})
	bundleLocal.AddTestInstance(&testing.TestInstance{
		Name:    "test.LocalFail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Fatalf("Failure")
		},
	})

	bundleRemote := testing.NewRegistry("bundle")
	bundleRemote.AddTestInstance(&testing.TestInstance{
		Name:    "test.Remote",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
		},
	})
	bundleRemote.AddTestInstance(&testing.TestInstance{
		Name:    "test.RemoteFail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Fatalf("Failure")
		},
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundleLocal),
		runtest.WithRemoteBundles(bundleRemote),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Repeats = 1
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New Failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("driver.ListMatchedTests Failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("driver.RunTests failed: %v", err)
	}

	// Repeats once after initial execution, so one
	// additional result for each test is expected
	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.Local",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.LocalFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.RemoteFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.LocalFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.RemoteFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
	}

	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}
}

func TestDriver_RunTests_RepeatsWithMaxFailures(t *gotesting.T) {
	bundleLocal := testing.NewRegistry("bundle")
	bundleLocal.AddTestInstance(&testing.TestInstance{
		Name:    "test.LocalFail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Fatalf("Failure")
		},
	})

	bundleRemote := testing.NewRegistry("bundle")
	bundleRemote.AddTestInstance(&testing.TestInstance{
		Name:    "test.RemoteFail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Fatalf("Failure")
		},
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundleLocal),
		runtest.WithRemoteBundles(bundleRemote),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Repeats = 1
		// 2 tests guaranteed to fail, run all tests twice
		// = should fail during the second repeat step, first execution.
		cfg.MaxTestFailures = 3
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New Failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("driver.ListMatchedTests Failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	// Expects error here.
	if err == nil {
		t.Error("RunTests unexpectedly succeeded")
	}

	// Should stop after 3rd failure as specified in Config.
	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.LocalFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.RemoteFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.LocalFail",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
	}

	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}
}

func TestDriver_RunTests_WithRetries(t *gotesting.T) {
	bundleLocal := testing.NewRegistry("bundle")

	bundleLocal.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local1",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failure")
		},
	})
	bundleLocal.AddTestInstance(&testing.TestInstance{
		Name:    "test.Local2",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	bundleRemote := testing.NewRegistry("bundle")

	bundleRemote.AddTestInstance(&testing.TestInstance{
		Name:    "test.Remote1",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failure")
		},
	})
	bundleRemote.AddTestInstance(&testing.TestInstance{
		Name:    "test.Remote2",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(bundleLocal),
		runtest.WithRemoteBundles(bundleRemote),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Retries = 1
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	got, err := drv.RunTests(ctx, tests, nil, nil, nil, nil)
	if err != nil {
		t.Errorf("RunTests failed: %v", err)
	}
	// Test results should only have 1 result per test in it...
	want := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.Local2",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},

		{
			Test: resultsjson.Test{
				Name:   "test.Remote2",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
	}
	if diff := cmp.Diff(got, want, resultsCmpOpts...); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%s", diff)
	}

	// The streamed json Results should show the retries.
	jsonWant := []*resultsjson.Result{
		{
			Test: resultsjson.Test{
				Name:   "test.Local1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local2",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Local1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote2",
				Bundle: "bundle",
			},
		},
		{
			Test: resultsjson.Test{
				Name:   "test.Remote1",
				Bundle: "bundle",
			},
			Errors: []resultsjson.Error{{Reason: "Failure"}},
		},
	}

	// Results in streamed_results.json.
	if b, err := os.ReadFile(filepath.Join(cfg.ResDir(), reporting.StreamedResultsFilename)); err != nil {
		t.Errorf("Failed to read %s: %v", reporting.StreamedResultsFilename, err)
	} else if results, err := unmarshalStreamedResults(b); err != nil {
		t.Errorf("Failed to parse %s: %v", reporting.StreamedResultsFilename, err)
	} else if diff := cmp.Diff(results, jsonWant, resultsCmpOpts...); diff != "" {
		t.Errorf("%s mismatch (-got +want):\n%s", reporting.StreamedResultsFilename, diff)
	}
}

func TestDriver_RunTests_TempDirs(t *gotesting.T) {
	tempRoot := testutil.TempDir(t)
	localTemp := filepath.Join(tempRoot, "local")
	remoteTemp := filepath.Join(tempRoot, "remote")

	localTest := &testing.TestInstance{
		Name:    "local.Tmp",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			if td := os.TempDir(); td != localTemp {
				t.Errorf("Unexpected local TMPDIR: got %s, want %s", td, localTemp)
			}
		},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest)

	remoteTest := &testing.TestInstance{
		Name:    "remote.Tmp",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			if td := os.TempDir(); td != remoteTemp {
				t.Errorf("Unexpected remote TMPDIR: got %s, want %s", td, remoteTemp)
			}
		},
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remoteTest)

	env := runtest.SetUp(
		t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.LocalTempDir = localTemp
		cfg.RemoteTempDir = remoteTemp
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	if _, err := drv.RunTests(ctx, tests, nil, nil, nil, nil); err != nil {
		t.Errorf("RunTests failed: %v", err)
	}
}

func TestDriver_RunTests_PushedFilesPaths(t *gotesting.T) {
	pushedFilesInfo := []*protocol.PushedFilesInfoForDUT{
		{
			Role: "",
			SrcDstPaths: map[string]string{
				"primary_src_1": "primary_dst_1",
				"primary_src_2": "primary_dst_2",
			},
		},
	}
	wanted := map[string]string{
		"primary_src_1": "primary_dst_1",
		"primary_src_2": "primary_dst_2",
	}

	var got map[string]string

	remoteTest := &testing.TestInstance{
		Name:    "remote.Tmp",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			got = s.PushedFilesToDUT("")
		},
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remoteTest)

	env := runtest.SetUp(
		t,
		runtest.WithRemoteBundles(remoteReg),
	)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	tests, err := drv.ListMatchedTests(ctx, nil)
	if err != nil {
		t.Fatalf("ListMatchedTests failed: %v", err)
	}

	if _, err := drv.RunTests(ctx, tests, nil, nil, nil, pushedFilesInfo); err != nil {
		t.Errorf("RunTests failed: %v", err)
	}
	if diff := cmp.Diff(got, wanted); diff != "" {
		t.Errorf("Information for pushed files mismatch (-got +want):\n%s", diff)
	}
}
