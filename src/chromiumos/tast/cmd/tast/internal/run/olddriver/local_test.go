// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package olddriver

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/testing/testfixture"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testutil"
)

func TestLocalSuccess(t *gotesting.T) {
	t.Parallel()

	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second) // avoid test being blocked indefinitely
	defer cancel()

	if _, err := RunLocalTests(ctx, cfg, state, nil, drv); err != nil {
		t.Errorf("RunLocalTests failed: %v", err)
	}
}

func TestLocalProxy(t *gotesting.T) {
	// Proxy environment variables should be prepended to the local_test_runner
	// command line.
	// (The variables are added in this order in local.go.)
	envVars := []struct{ Name, Value string }{
		{"HTTP_PROXY", "10.0.0.1:8000"},
		{"HTTPS_PROXY", "10.0.0.1:8001"},
		{"NO_PROXY", "foo.com, localhost, 127.0.0.0"},
	}

	// Configure proxy settings to forward to the DUT.
	for _, v := range envVars {
		old, ok := os.LookupEnv(v.Name)
		if err := os.Setenv(v.Name, v.Value); err != nil {
			t.Fatal(err)
		}
		if !ok {
			defer os.Unsetenv(v.Name)
		} else {
			defer os.Setenv(v.Name, old)
		}
	}

	envPairs := make([]string, len(envVars))
	for i, v := range envVars {
		envPairs[i] = fmt.Sprintf("%s=%s", v.Name, v.Value)
	}

	expCmd := fmt.Sprintf("exec env %s %s", shutil.EscapeSlice(envPairs), runtest.LocalTestRunnerPath)
	called := false
	fakeProc := func(_ io.Reader, _, _ io.Writer) int {
		called = true
		return 1
	}

	env := runtest.SetUp(t, runtest.WithExtraSSHHandlers([]fakesshserver.Handler{
		fakesshserver.ExactMatchHandler(expCmd, fakeProc),
		fakesshserver.ExactMatchHandler(expCmd+" -rpc", fakeProc),
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Proxy = config.ProxyEnv
	})
	state := env.State()

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if _, err := RunLocalTests(ctx, cfg, state, nil, drv); err == nil {
		t.Error("RunLocalTests unexpectedly succeeded")
	}
	if !called {
		t.Error("local_test_runner was not called with expected environment variables")
	}
}

func TestLocalCopyOutput(t *gotesting.T) {
	const (
		testName = "pkg.Test"
		outFile  = "somefile.txt"
		outData  = "somedata"
	)

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Name:    testName,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			ioutil.WriteFile(filepath.Join(s.OutDir(), outFile), []byte(outData), 0644)
		},
	})

	env := runtest.SetUp(t, runtest.WithLocalBundles(reg))
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	state.TestsToRun = []*driver.BundleEntity{
		{Bundle: "bundle", Resolved: &protocol.ResolvedEntity{Entity: &protocol.Entity{Name: testName}, Hops: 1}},
	}

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if _, err := RunLocalTests(ctx, cfg, state, nil, drv); err != nil {
		t.Fatalf("RunLocalTests failed: %v", err)
	}

	files, err := testutil.ReadFiles(filepath.Join(cfg.ResDir(), testLogsDir))
	if err != nil {
		t.Fatal(err)
	}
	if out, ok := files[filepath.Join(testName, outFile)]; !ok {
		t.Errorf("%s was not created", filepath.Join(testName, outFile))
	} else if out != outData {
		t.Errorf("%s was corrupted: got %q, want %q", filepath.Join(testName, outFile), out, outData)
	}
}

// TestLocalMaxFailures makes sure that RunLocalTests does not run any tests after maximum failures allowed has been reached.
func TestLocalMaxFailures(t *gotesting.T) {
	const (
		testName1 = "pkg.Test1"
		testName2 = "pkg.Test2"
		testName3 = "pkg.Test3"
	)

	reg := testing.NewRegistry("bundle")
	for _, testName := range []string{testName1, testName2, testName3} {
		reg.AddTestInstance(&testing.TestInstance{
			Name:    testName,
			Timeout: time.Minute,
			Func: func(ctx context.Context, s *testing.State) {
				s.Error("Error 1")
				s.Error("Error 2")
				s.Error("Error 3")
			},
		})
	}

	env := runtest.SetUp(t, runtest.WithLocalBundles(reg))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.MaxTestFailures = 2
	})

	state := env.State()
	state.TestsToRun = []*driver.BundleEntity{
		{Bundle: "bundle", Resolved: &protocol.ResolvedEntity{Entity: &protocol.Entity{Name: testName1}, Hops: 1}},
		{Bundle: "bundle", Resolved: &protocol.ResolvedEntity{Entity: &protocol.Entity{Name: testName2}, Hops: 1}},
		{Bundle: "bundle", Resolved: &protocol.ResolvedEntity{Entity: &protocol.Entity{Name: testName3}, Hops: 1}},
	}

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	results, err := RunLocalTests(ctx, cfg, state, nil, drv)
	if err == nil {
		t.Errorf("RunLocalTests() passed unexpectedly")
	}
	if len(results) != 2 {
		t.Errorf("RunLocalTests return %v results; want 2", len(results))
	}
}

func TestFixturesDependency(t *gotesting.T) {
	remoteFixtures := []*testing.FixtureInstance{
		{
			Name: "remoteFixt",
			Impl: testfixture.New(
				testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
					s.Log("Hello")
					return nil
				}),
				testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
					s.Log("Bye")
				}),
			),
		},
		{
			Name: "failFixt",
			Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Whoa")
				return nil
			})),
		},
		{
			Name: "tearDownFailFixt",
			Impl: testfixture.New(testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				s.Error("Oops")
			})),
		},
		// The same fixture might be accidentally linked to both local
		// test bundles and remote test bundles. In this case, a remote
		// fixture is ignored (crbug.com/1179162).
		{
			Name: "fixt1B",
			Impl: testfixture.New(),
		},
	}
	localFixtures := []*testing.FixtureInstance{
		{Name: "fixt1B", Parent: "remoteFixt", Impl: testfixture.New()},
		{Name: "fixt2", Parent: "failFixt", Impl: testfixture.New()},
		{Name: "fixt3A", Parent: "localFixt", Impl: testfixture.New()},
		{Name: "fixt3B", Impl: testfixture.New()},
		{Name: "localFixt", Impl: testfixture.New()},
	}
	nop := func(context.Context, *testing.State) {}
	localTests := []*testing.TestInstance{
		{Name: "pkg.Test1A", Fixture: "remoteFixt", Func: nop},
		{Name: "pkg.Test1B", Fixture: "fixt1B", Func: nop}, // depends on remoteFixt
		{Name: "pkg.Test2", Fixture: "fixt2", Func: nop},   // depends on failFixt
		{Name: "pkg.Test3A", Fixture: "fixt3A", Func: nop}, // depends on localFixt
		{Name: "pkg.Test3B", Fixture: "fixt3B", Func: nop}, // depends on nothing
		{Name: "pkg.Test3C", Fixture: "", Func: nop},
		{Name: "pkg.Test4", Fixture: "tearDownFailFixt", Func: nop},
	}
	remoteTests := []*testing.TestInstance{
		// Remote tests are not used on computing fixtures to run.
		{Name: "pkg.RemoteTest", Fixture: "shouldNotRun", Func: nop},
	}

	makeReg := func(tests []*testing.TestInstance, fixtures []*testing.FixtureInstance) *testing.Registry {
		reg := testing.NewRegistry("bundle")
		for _, t := range tests {
			reg.AddTestInstance(t)
		}
		for _, f := range fixtures {
			reg.AddFixtureInstance(f)
		}
		return reg
	}

	localReg := makeReg(localTests, localFixtures)
	remoteReg := makeReg(remoteTests, remoteFixtures)

	env := runtest.SetUp(t, runtest.WithLocalBundles(localReg), runtest.WithRemoteBundles(remoteReg))
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	for _, t := range localTests {
		state.TestsToRun = append(state.TestsToRun, &driver.BundleEntity{
			Bundle: "bundle",
			Resolved: &protocol.ResolvedEntity{
				Entity: t.EntityProto(),
				Hops:   1,
			},
		})
	}
	for _, t := range remoteTests {
		state.TestsToRun = append(state.TestsToRun, &driver.BundleEntity{
			Bundle: "bundle",
			Resolved: &protocol.ResolvedEntity{
				Entity: t.EntityProto(),
				Hops:   0,
			},
		})
	}

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	got, err := RunLocalTests(ctx, cfg, state, nil, drv)
	if err != nil {
		t.Fatalf("RunLocalTests: %v", err)
	}

	want := []*resultsjson.Result{
		// First, local tests not depending on remote fixtures are run.
		{Test: resultsjson.Test{Name: "pkg.Test3C", Bundle: "bundle"}},
		{Test: resultsjson.Test{Name: "pkg.Test3B", Bundle: "bundle"}},
		{Test: resultsjson.Test{Name: "pkg.Test3A", Bundle: "bundle"}},
		// Next, we run local tests depending on the remote fixture failFixt.
		// Since the fixture fails to set up, these tests fail immediately.
		{Test: resultsjson.Test{Name: "pkg.Test2", Bundle: "bundle"}, Errors: []resultsjson.Error{{Reason: "[Fixture failure] failFixt: Whoa"}}},
		// Next, we run local tests depending on the remote fixture remoteFixt.
		{Test: resultsjson.Test{Name: "pkg.Test1A", Bundle: "bundle"}},
		{Test: resultsjson.Test{Name: "pkg.Test1B", Bundle: "bundle"}},
		// Finally, we run local tests depending on the remote fixture
		// tearDownFailFixt. Though the fixture fails to tear down, its failure
		// is invisible.
		{Test: resultsjson.Test{Name: "pkg.Test4", Bundle: "bundle"}},
	}

	opts := cmp.Options{
		cmpopts.IgnoreFields(resultsjson.Result{}, "Start", "End", "OutDir"),
		cmpopts.IgnoreFields(resultsjson.Test{}, "Fixture", "Timeout"),
		cmpopts.IgnoreFields(resultsjson.Error{}, "Time", "File", "Line", "Stack"),
	}
	if diff := cmp.Diff(got, want, opts); diff != "" {
		t.Errorf("Results mismatch (-got +want):\n%v", diff)
	}
}
