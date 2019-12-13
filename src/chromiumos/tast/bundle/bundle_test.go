// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/dut"
	"chromiumos/tast/host/test"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

var testFunc = func(context.Context, *testing.State) {}

// testPre implements both Precondition and preconditionImpl for unit tests.
// TODO(derat): This is duplicated from tast/testing/test_test.go. Find a common location.
type testPre struct {
	prepareFunc func(context.Context, *testing.State) interface{}
	closeFunc   func(context.Context, *testing.State)
	name        string // name to return from String
}

func (p *testPre) Prepare(ctx context.Context, s *testing.State) interface{} {
	if p.prepareFunc != nil {
		return p.prepareFunc(ctx, s)
	}
	return nil
}

func (p *testPre) Close(ctx context.Context, s *testing.State) {
	if p.closeFunc != nil {
		p.closeFunc(ctx, s)
	}
}

func (p *testPre) Timeout() time.Duration { return time.Minute }

func (p *testPre) String() string { return p.name }

// errorHasStatus returns true if err is of type *command.StatusError and contains the supplied status code.
func errorHasStatus(err error, status int) bool {
	if se, ok := err.(*command.StatusError); !ok {
		return false
	} else if se.Status() != status {
		return false
	}
	return true
}

func TestCopyTestOutput(t *gotesting.T) {
	const msg = "here is a log message"
	e := testing.Error{
		Reason: "something went wrong",
		File:   "file.go",
		Line:   16,
		Stack:  "the stack",
	}
	t1 := time.Unix(1, 0)
	t2 := time.Unix(2, 0)

	ch := make(chan testing.Output)
	go func() {
		ch <- testing.Output{T: t1, Msg: msg}
		ch <- testing.Output{T: t2, Err: &e}
		close(ch)
	}()

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	copyTestOutput(ch, newEventWriter(mw), make(chan bool))

	r := control.NewMessageReader(&b)
	for i, em := range []interface{}{
		&control.TestLog{Time: t1, Text: msg},
		&control.TestError{Time: t2, Error: e},
	} {
		if am, err := r.ReadMessage(); err != nil {
			t.Errorf("Failed to read message %v: %v", i, err)
		} else if !cmp.Equal(am, em) {
			t.Errorf("Message %v is %v; want %v", i, am, em)
		}
	}
	if r.More() {
		t.Error("copyTestOutput() wrote extra message(s)")
	}
}

func TestCopyTestOutputTimeout(t *gotesting.T) {
	// Simulate a test ignoring its timeout by requesting abort and leaving the output channel open.
	abort := make(chan bool, 1)
	abort <- true
	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	copyTestOutput(make(chan testing.Output), newEventWriter(mw), abort)

	r := control.NewMessageReader(&b)
	if msg, err := r.ReadMessage(); err != nil {
		t.Errorf("Failed to read message: %v", err)
	} else if _, ok := msg.(*control.TestError); !ok {
		t.Errorf("copyTestOutput() wrote %v; want TestError", msg)
	}

	foundMe := false
	for r.More() {
		msg, err := r.ReadMessage()
		if err != nil {
			t.Error("Failed to read message: ", err)
			break
		}
		log, ok := msg.(*control.TestLog)
		if !ok {
			t.Errorf("Got a message of %T, want *control.TestLog", msg)
			continue
		}
		// The log should contain stack traces, including this test function.
		if strings.Contains(log.Text, "TestCopyTestOutputTimeout") {
			foundMe = true
		}
	}
	if !foundMe {
		t.Error("Did not find a stack trace containing TestCopyTestOutputTimeout in logs")
	}
}

func TestRunTests(t *gotesting.T) {
	const (
		name1           = "foo.Test1"
		name2           = "foo.Test2"
		preRunMsg       = "setting up for run"
		postRunMsg      = "cleaning up after run"
		preTestMsg      = "setting up for test"
		postTestMsg     = "cleaning up for test"
		postTestHookMsg = "setup hook for test"
	)

	reg := testing.NewRegistry()
	reg.AddTestCase(&testing.TestCase{
		Name:    name1,
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute},
	)
	reg.AddTestCase(&testing.TestCase{
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
	tests := reg.AllTests()
	var preRunCalls, postRunCalls, preTestCalls, postTestCalls, postTestHookCalls int
	args := Args{
		RunTests: &RunTestsArgs{
			OutDir:  tmpDir,
			DataDir: tmpDir,
			TempDir: runTmpDir,
		},
	}
	cfg := runConfig{
		preRunFunc: func(ctx context.Context, lf logFunc) (context.Context, error) {
			preRunCalls++
			lf(preRunMsg)
			return ctx, nil
		},
		postRunFunc: func(ctx context.Context, lf logFunc) error {
			postRunCalls++
			lf(postRunMsg)
			return nil
		},
		preTestFunc: func(ctx context.Context, s *testing.State) func(ctx context.Context, s *testing.State) {
			preTestCalls++
			s.Log(preTestMsg)

			return func(ctx context.Context, s *testing.State) {
				postTestHookCalls++
				s.Log(postTestHookMsg)
			}
		},
		postTestFunc: func(ctx context.Context, s *testing.State) {
			postTestCalls++
			s.Log(postTestMsg)
		},
	}

	sig := fmt.Sprintf("runTests(..., %+v, %+v)", args, cfg)
	if err := runTests(context.Background(), &stdout, &args, &cfg, localBundle, tests); err != nil {
		t.Fatalf("%v failed: %v", sig, err)
	}

	if preRunCalls != 1 {
		t.Errorf("%v called pre-run function %d time(s); want 1", sig, preRunCalls)
	}
	if postRunCalls != 1 {
		t.Errorf("%v called run post-run function %d time(s); want 1", sig, postRunCalls)
	}
	if preTestCalls != len(tests) {
		t.Errorf("%v called pre-test function %d time(s); want %d", sig, preTestCalls, len(tests))
	}
	if postTestCalls != len(tests) {
		t.Errorf("%v called post-test function %d time(s); want %d", sig, postTestCalls, len(tests))
	}
	if postTestHookCalls != len(tests) {
		t.Errorf("%v called post-test-hook function %d time(s); want %d", sig, postTestHookCalls, len(tests))
	}

	// Just check some basic details of the control messages.
	r := control.NewMessageReader(&stdout)
	for i, ei := range []interface{}{
		&control.RunLog{Text: preRunMsg},
		&control.TestStart{Test: *tests[0]},
		&control.TestLog{Text: preTestMsg},
		&control.TestLog{Text: postTestMsg},
		&control.TestLog{Text: postTestHookMsg},
		&control.TestEnd{Name: name1},
		&control.TestStart{Test: *tests[1]},
		&control.TestLog{Text: preTestMsg},
		&control.TestError{},
		&control.TestLog{Text: postTestMsg},
		&control.TestLog{Text: postTestHookMsg},
		&control.TestEnd{Name: name2},
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
			case *control.TestStart:
				if am, ok := ai.(*control.TestStart); !ok {
					t.Errorf("Got %v at %d; want TestStart", ai, i)
				} else if am.Test.Name != em.Test.Name {
					t.Errorf("Got TestStart with Test %q at %d; want %q", am.Test.Name, i, em.Test.Name)
				}
			case *control.TestEnd:
				if am, ok := ai.(*control.TestEnd); !ok {
					t.Errorf("Got %v at %d; want TestEnd", ai, i)
				} else if am.Name != em.Name {
					t.Errorf("Got TestEnd for %q at %d; want %q", am.Name, i, em.Name)
				} else if am.TimingLog == nil {
					t.Error("Got TestEnd with missing timing log at ", i)
				}
			case *control.TestError:
				if _, ok := ai.(*control.TestError); !ok {
					t.Errorf("Got %v at %d; want TestError", ai, i)
				}
			case *control.TestLog:
				if am, ok := ai.(*control.TestLog); !ok {
					t.Errorf("Got %v at %d; want TestLog", ai, i)
				} else if am.Text != em.Text {
					t.Errorf("Got TestLog containing %q at %d; want %q", am.Text, i, em.Text)
				}
			}
		}
	}
	if r.More() {
		t.Errorf("%v wrote extra message(s)", sig)
	}
}

func TestRunTestsTimeout(t *gotesting.T) {
	reg := testing.NewRegistry()

	// The first test blocks indefinitely on a channel.
	const name1 = "foo.Test1"
	ch := make(chan bool, 1)
	defer func() { ch <- true }()
	reg.AddTestCase(&testing.TestCase{
		Name:        name1,
		Func:        func(context.Context, *testing.State) { <-ch },
		Timeout:     10 * time.Millisecond,
		ExitTimeout: time.Millisecond, // avoid blocking after timeout
	})

	// The second test blocks for 50 ms and specifies a custom one-minute timeout.
	const name2 = "foo.Test2"
	reg.AddTestCase(&testing.TestCase{
		Name:    name2,
		Func:    func(context.Context, *testing.State) { time.Sleep(50 * time.Millisecond) },
		Timeout: time.Minute,
	})

	stdout := bytes.Buffer{}
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	args := Args{
		RunTests: &RunTestsArgs{
			OutDir:  tmpDir,
			DataDir: tmpDir,
		},
	}

	// The first test should time out after 10 milliseconds.
	// The second test should succeed since it finishes before its timeout.
	if err := runTests(context.Background(), &stdout, &args, &runConfig{}, localBundle, reg.AllTests()); err != nil {
		t.Fatalf("runTests(..., %+v, ...) failed: %v", args, err)
	}

	var name string             // name of current test
	errors := make([]string, 0) // name of test from each error
	r := control.NewMessageReader(&stdout)
	for r.More() {
		if msg, err := r.ReadMessage(); err != nil {
			t.Error("ReadMessage failed: ", err)
		} else if ts, ok := msg.(*control.TestStart); ok {
			name = ts.Test.Name
		} else if _, ok := msg.(*control.TestError); ok {
			errors = append(errors, name)
		}
	}
	exp := []string{name1}
	if !reflect.DeepEqual(errors, exp) {
		t.Errorf("Got errors %v; wanted %v", errors, exp)
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	// runTests should report failure when passed an empty slice of tests.
	if err := runTests(context.Background(), &bytes.Buffer{}, &Args{RunTests: &RunTestsArgs{}},
		&runConfig{}, localBundle, []*testing.TestCase{}); !errorHasStatus(err, statusNoTests) {
		t.Fatalf("runTests() = %v; want status %v", err, statusNoTests)
	}
}

func TestRunTestsMissingDeps(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	const (
		validName   = "foo.Valid"
		missingName = "foo.Missing"
		unregName   = "foo.Unregistered"

		validDep   = "valid"
		missingDep = "missing"
		unregDep   = "unreg"
	)

	// Register three tests: one with a satisfied dep, another with a missing dep,
	// and a third with an unregistered dep.
	testRan := make(map[string]bool)
	makeFunc := func(name string) testing.TestFunc {
		return func(context.Context, *testing.State) { testRan[name] = true }
	}
	testing.AddTestCase(&testing.TestCase{Name: validName, Func: makeFunc(validName), SoftwareDeps: []string{validDep}})
	testing.AddTestCase(&testing.TestCase{Name: missingName, Func: makeFunc(missingName), SoftwareDeps: []string{missingDep}})
	testing.AddTestCase(&testing.TestCase{Name: unregName, Func: makeFunc(unregName), SoftwareDeps: []string{unregDep}})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := Args{
		Mode: RunTestsMode,
		RunTests: &RunTestsArgs{
			OutDir:                      tmpDir,
			DataDir:                     tmpDir,
			CheckSoftwareDeps:           true,
			AvailableSoftwareFeatures:   []string{validDep},
			UnavailableSoftwareFeatures: []string{missingDep},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, &Args{},
		&runConfig{defaultTestTimeout: time.Minute}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// Read through the control messages to get test results.
	var testName string
	testFailed := make(map[string][]testing.Error)
	testMissingDeps := make(map[string][]string)
	r := control.NewMessageReader(stdout)
	for r.More() {
		msg, err := r.ReadMessage()
		if err != nil {
			t.Fatal("Failed to read message:", err)
		}
		switch m := msg.(type) {
		case *control.TestStart:
			testName = m.Test.Name
		case *control.TestEnd:
			testMissingDeps[testName] = m.MissingSoftwareDeps
		case *control.TestError:
			testFailed[testName] = append(testFailed[testName], m.Error)
		}
	}

	// Verify that the expected results were reported for each test.
	for _, tc := range []struct {
		name        string
		shouldRun   bool
		shouldFail  bool
		missingDeps []string
	}{
		{validName, true, false, nil},
		{missingName, false, false, []string{missingDep}},
		{unregName, false, true, []string{unregDep}},
	} {
		if testRan[tc.name] && !tc.shouldRun {
			t.Errorf("%v ran unexpectedly", tc.name)
		} else if !testRan[tc.name] && tc.shouldRun {
			t.Errorf("%v didn't run", tc.name)
		}
		if _, failed := testFailed[tc.name]; failed && !tc.shouldFail {
			t.Errorf("%v failed: %+v", tc.name, testFailed[tc.name])
		} else if !failed && tc.shouldFail {
			t.Errorf("%v didn't fail", tc.name)
		}
		if !reflect.DeepEqual(testMissingDeps[tc.name], tc.missingDeps) {
			t.Errorf("%v had missing deps %v; want %v",
				tc.name, testMissingDeps[tc.name], tc.missingDeps)
		}
	}
}

func TestRunTestsSkipTestWithPrecondition(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	var actions []string
	makePre := func(name string) *testPre {
		return &testPre{
			prepareFunc: func(context.Context, *testing.State) interface{} {
				actions = append(actions, "prepare_"+name)
				return nil
			},
			closeFunc: func(context.Context, *testing.State) { actions = append(actions, "close_"+name) },
			name:      name,
		}
	}
	pre1 := makePre("pre1")
	pre2 := makePre("pre2")

	// Make the last test using each precondition get skipped due to unsatisfied dependencies.
	f := func(context.Context, *testing.State) {}
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test1", Func: f, Pre: pre1})
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test2", Func: f, Pre: pre1})
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test3", Func: f, Pre: pre1, SoftwareDeps: []string{"dep"}})
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test4", Func: f, Pre: pre2})
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test5", Func: f, Pre: pre2})
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test6", Func: f, Pre: pre2, SoftwareDeps: []string{"dep"}})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := Args{
		Mode: RunTestsMode,
		RunTests: &RunTestsArgs{
			OutDir:                      tmpDir,
			DataDir:                     tmpDir,
			CheckSoftwareDeps:           true,
			UnavailableSoftwareFeatures: []string{"dep"},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, &Args{},
		&runConfig{defaultTestTimeout: time.Minute}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// We should've still closed each precondition after running the last test that needs it: https://crbug.com/950499
	exp := []string{"prepare_pre1", "prepare_pre1", "close_pre1", "prepare_pre2", "prepare_pre2", "close_pre2"}
	if !reflect.DeepEqual(actions, exp) {
		t.Errorf("run() performed actions %v; want %v", actions, exp)
	}
}

func TestRunRemoteData(t *gotesting.T) {
	td := test.NewTestData(userKey, hostKey, nil)
	defer td.Close()

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	var (
		meta *testing.Meta
		hint *testing.RPCHint
		dt   *dut.DUT
	)
	testing.AddTestCase(&testing.TestCase{
		Name: "meta.Test",
		Func: func(ctx context.Context, s *testing.State) {
			meta = s.Meta()
			hint = s.RPCHint()
			dt = s.DUT()
		},
	})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := Args{
		Mode: RunTestsMode,
		RunTests: &RunTestsArgs{
			OutDir:         tmpDir,
			DataDir:        tmpDir,
			TastPath:       "/bogus/tast",
			Target:         td.Srv.Addr().String(),
			KeyFile:        td.UserKeyFile,
			RunFlags:       []string{"-flag1", "-flag2"},
			LocalBundleDir: "/mock/local/bundles",
		},
	}
	stdin := newBufferWithArgs(t, &args)
	if status := run(context.Background(), nil, stdin, &bytes.Buffer{}, &bytes.Buffer{}, &Args{},
		&runConfig{defaultTestTimeout: time.Minute}, remoteBundle); status != statusSuccess {
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
	expHint := &testing.RPCHint{
		LocalBundleDir: args.RunTests.LocalBundleDir,
	}
	if !reflect.DeepEqual(hint, expHint) {
		t.Errorf("Test got RPCHint %+v; want %+v", *hint, *expHint)
	}
	if dt == nil {
		t.Error("DUT is not available")
	}
}

func TestRunList(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	f := func(context.Context, *testing.State) {}
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test", Func: f})
	testing.AddTestCase(&testing.TestCase{Name: "pkg.Test2", Func: f})

	var exp bytes.Buffer
	if err := testing.WriteTestsAsJSON(&exp, testing.GlobalRegistry().AllTests()); err != nil {
		t.Fatal(err)
	}

	// ListTestsMode should result in tests being JSON-marshaled to stdout.
	stdin := newBufferWithArgs(t, &Args{Mode: ListTestsMode, ListTests: &ListTestsArgs{}})
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{},
		&Args{}, &runConfig{}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run() wrote %q; want %q", stdout.String(), exp.String())
	}

	// The -dumptests command-line flag should do the same thing.
	clArgs := []string{"-dumptests"}
	stdout.Reset()
	if status := run(context.Background(), clArgs, &bytes.Buffer{}, stdout, &bytes.Buffer{},
		&Args{}, &runConfig{}, localBundle); status != statusSuccess {
		t.Fatalf("run(%v) returned status %v; want %v", clArgs, status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run(%v) wrote %q; want %q", clArgs, stdout.String(), exp.String())
	}
}

func TestRunRegistrationError(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	const name = "cat.MyTest"
	testing.AddTestCase(&testing.TestCase{Name: name, Func: testFunc})

	// Adding a test with same name should generate an error.
	testing.AddTestCase(&testing.TestCase{Name: name, Func: testFunc})

	stdin := newBufferWithArgs(t, &Args{Mode: ListTestsMode, ListTests: &ListTestsArgs{}})
	if status := run(context.Background(), nil, stdin, ioutil.Discard, ioutil.Discard,
		&Args{}, &runConfig{}, localBundle); status != statusBadTests {
		t.Errorf("run() with bad test returned status %v; want %v", status, statusBadTests)
	}
}

func TestTestsToRunSortTests(t *gotesting.T) {
	const (
		test1 = "pkg.Test1"
		test2 = "pkg.Test2"
		test3 = "pkg.Test3"
	)

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestCase(&testing.TestCase{Name: test2, Func: testFunc})
	testing.AddTestCase(&testing.TestCase{Name: test3, Func: testFunc})
	testing.AddTestCase(&testing.TestCase{Name: test1, Func: testFunc})

	tests, err := testsToRun(&runConfig{}, nil)
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

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddTestCase(&testing.TestCase{Name: name1, Func: testFunc, Timeout: customTimeout})
	testing.AddTestCase(&testing.TestCase{Name: name2, Func: testFunc})

	tests, err := testsToRun(&runConfig{defaultTestTimeout: defaultTimeout}, nil)
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
