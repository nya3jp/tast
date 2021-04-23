// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

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
	)

	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{
		Name:    name1,
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute},
	)
	reg.AddTestInstance(&testing.TestInstance{
		Name:    name2,
		Func:    func(ctx context.Context, s *testing.State) { s.Error("error") },
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

	stdout := bytes.Buffer{}
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
			testcontext.Log(ctx, preRunMsg)
			return func(ctx context.Context) error {
				postRunCalls++
				testcontext.Log(ctx, postRunMsg)
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

	sig := fmt.Sprintf("runTests(..., %+v, %+v)", *cfg, *scfg)
	if err := runTests(context.Background(), &stdout, cfg, scfg); err != nil {
		t.Fatalf("%v failed: %v", sig, err)
	}

	if preRunCalls != 1 {
		t.Errorf("%v called pre-run function %d time(s); want 1", sig, preRunCalls)
	}
	if postRunCalls != 1 {
		t.Errorf("%v called run post-run function %d time(s); want 1", sig, postRunCalls)
	}

	tests := reg.AllTests()
	if preTestCalls != len(tests) {
		t.Errorf("%v called pre-test function %d time(s); want %d", sig, preTestCalls, len(tests))
	}
	if postTestCalls != len(tests) {
		t.Errorf("%v called post-test function %d time(s); want %d", sig, postTestCalls, len(tests))
	}

	// Just check some basic details of the control messages.
	r := control.NewMessageReader(&stdout)
	for i, ei := range []interface{}{
		&control.RunLog{Text: preRunMsg},
		&control.RunLog{Text: "Devserver status: using pseudo client"},
		&control.RunLog{Text: "Found 0 external linked data file(s), need to download 0"},
		&control.EntityStart{Info: *jsonprotocol.MustEntityInfoFromProto(tests[0].EntityProto())},
		&control.EntityLog{Text: preTestMsg},
		&control.EntityLog{Text: postTestMsg},
		&control.EntityEnd{Name: name1},
		&control.EntityStart{Info: *jsonprotocol.MustEntityInfoFromProto(tests[1].EntityProto())},
		&control.EntityLog{Text: preTestMsg},
		&control.EntityError{},
		&control.EntityLog{Text: postTestMsg},
		&control.EntityEnd{Name: name2},
		&control.RunLog{Text: postRunMsg},
	} {
		if ai, err := r.ReadMessage(); err != nil {
			t.Errorf("Failed to read message %d: %v", i, err)
		} else {
			switch em := ei.(type) {
			case *control.RunLog:
				if am, ok := ai.(*control.RunLog); !ok {
					t.Errorf("Got %v at %d; want RunLog", ai, i)
				} else if am.Text != em.Text {
					t.Errorf("Got RunLog containing %q at %d; want %q", am.Text, i, em.Text)
				}
			case *control.EntityStart:
				if am, ok := ai.(*control.EntityStart); !ok {
					t.Errorf("Got %v at %d; want EntityStart", ai, i)
				} else if am.Info.Name != em.Info.Name {
					t.Errorf("Got EntityStart with Test %q at %d; want %q", am.Info.Name, i, em.Info.Name)
				}
			case *control.EntityEnd:
				if am, ok := ai.(*control.EntityEnd); !ok {
					t.Errorf("Got %v at %d; want EntityEnd", ai, i)
				} else if am.Name != em.Name {
					t.Errorf("Got EntityEnd for %q at %d; want %q", am.Name, i, em.Name)
				} else if am.TimingLog == nil {
					t.Error("Got EntityEnd with missing timing log at ", i)
				}
			case *control.EntityError:
				if _, ok := ai.(*control.EntityError); !ok {
					t.Errorf("Got %v at %d; want EntityError", ai, i)
				}
			case *control.EntityLog:
				if am, ok := ai.(*control.EntityLog); !ok {
					t.Errorf("Got %v at %d; want EntityLog", ai, i)
				} else if am.Text != em.Text {
					t.Errorf("Got EntityLog containing %q at %d; want %q", am.Text, i, em.Text)
				}
			}
		}
	}
	if r.More() {
		t.Errorf("%v wrote extra message(s)", sig)
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	// runTests should report success when no test is executed.
	if err := runTests(context.Background(), &bytes.Buffer{}, nil, NewStaticConfig(testing.NewRegistry(), 0, Delegate{})); err != nil {
		t.Fatalf("runTests failed for empty tests: %v", err)
	}
}

func TestRunRemoteData(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	reg := testing.NewRegistry()

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

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
			OutDir:         tmpDir,
			DataDir:        tmpDir,
			TastPath:       "/bogus/tast",
			Target:         td.Srvs[0].Addr().String(),
			KeyFile:        td.UserKeyFile,
			RunFlags:       []string{"-flag1", "-flag2"},
			LocalBundleDir: "/mock/local/bundles",
			FeatureArgs: jsonprotocol.FeatureArgs{
				TestVars: map[string]string{"var1": "value1"},
			},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	if status := run(context.Background(), nil, stdin, &bytes.Buffer{}, &bytes.Buffer{}, NewStaticConfig(reg, time.Minute, Delegate{})); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// The test should have access to information related to remote tests.
	expMeta := &testing.Meta{
		TastPath: args.RunTests.TastPath,
		Target:   args.RunTests.Target,
		RunFlags: args.RunTests.RunFlags,
	}
	if !reflect.DeepEqual(meta, expMeta) {
		t.Errorf("Test got Meta %+v; want %+v", *meta, *expMeta)
	}
	expHint := testing.NewRPCHint(args.RunTests.LocalBundleDir, args.RunTests.TestVars)
	if !reflect.DeepEqual(hint, expHint) {
		t.Errorf("Test got RPCHint %+v; want %+v", *hint, *expHint)
	}
	if dt == nil {
		t.Error("DUT is not available")
	}
}

func TestRunStartFixture(t *gotesting.T) {
	const testName = "pkg.Test"
	// runTests should not run runHook if tests depend on remote fixtures.
	// TODO(crbug/1184567): consider long term plan about interactions between
	// remote fixtures and run hooks.
	cfg := &protocol.RunConfig{
		Tests:             []string{testName},
		StartFixtureState: &protocol.StartFixtureState{Name: "foo"},
	}
	reg := testing.NewRegistry()
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
	if err := runTests(context.Background(), &bytes.Buffer{}, cfg, scfg); err != nil {
		t.Fatalf("runTests(): %v", err)
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
	if err := runTests(context.Background(), &bytes.Buffer{}, cfg, scfg); err != nil {
		t.Fatalf("runTests(): %v", err)
	}
	if !called {
		t.Error("runHook was not called")
	}
}

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

func TestRemoteCantConnect(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(context.Context, *testing.State) {}})

	// Remote should fail if the initial connection to the DUT couldn't be
	// established since the user key wasn't passed.
	args := jsonprotocol.BundleArgs{
		Mode:     jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{Target: td.Srvs[0].Addr().String()},
	}
	stderr := bytes.Buffer{}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &stderr, reg, Delegate{}); status != statusError {
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
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(ctx context.Context, s *testing.State) {
		dt := s.DUT()
		out, err := dt.Command(cmd).Output(ctx)
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
			OutDir:  outDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
		},
	}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &bytes.Buffer{}, reg, Delegate{}); status != statusSuccess {
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
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: makeFunc(&conn1)})
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test2", Func: makeFunc(&conn2)})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
			OutDir:  outDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
		},
	}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &bytes.Buffer{}, reg, Delegate{}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
	}
	if conn1 != true {
		t.Errorf("Remote(%+v) didn't pass live connection to first test", args)
	}
	if conn2 != true {
		t.Errorf("Remote(%+v) didn't pass live connection to second test", args)
	}
}

// TestBeforeReboot makes sure hook function is called before reboot.
func TestBeforeReboot(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()
	reg := testing.NewRegistry()

	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: func(ctx context.Context, s *testing.State) {
		s.DUT().Reboot(ctx)
		s.DUT().Reboot(ctx)
	}})

	// Set up test argument.
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
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
	if status := Remote(nil, stdin, &bytes.Buffer{}, &stderr, reg, Delegate{
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
	reg := testing.NewRegistry()
	const role = "role"
	reg.AddTestInstance(&testing.TestInstance{Name: "pkg.Test", Func: func(ctx context.Context, s *testing.State) {
		dt := s.CompanionDUT(role)
		out, err := dt.Command(cmd).Output(ctx)
		if err != nil {
			s.Fatalf("Got error when running %q: %v", cmd, err)
		}
		realOutput = string(out)
	}})

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)
	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRunTestsMode,
		RunTests: &jsonprotocol.BundleRunTestsArgs{
			OutDir:        outDir,
			Target:        td.Srvs[0].Addr().String(),
			CompanionDUTs: map[string]string{role: companionHost.Addr().String()},
			KeyFile:       td.UserKeyFile,
		},
	}
	if status := Remote(nil, newBufferWithArgs(t, &args), &bytes.Buffer{}, &bytes.Buffer{}, reg, Delegate{}); status != statusSuccess {
		t.Errorf("Remote(%+v) = %v; want %v", args, status, statusSuccess)
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

	reg := testing.NewRegistry()
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

	reg := testing.NewRegistry()
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
