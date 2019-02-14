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
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/command"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

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
	copyTestOutput(ch, newEventWriter(&b), make(chan bool))

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
	copyTestOutput(make(chan testing.Output), newEventWriter(&b), abort)

	r := control.NewMessageReader(&b)
	if msg, err := r.ReadMessage(); err != nil {
		t.Errorf("Failed to read message: %v", err)
	} else if _, ok := msg.(*control.TestError); !ok {
		t.Errorf("copyTestOutput() wrote %v; want TestError", msg)
	}
	if r.More() {
		t.Error("copyTestOutput() wrote extra message(s)")
	}
}

func TestRunTests(t *gotesting.T) {
	const (
		name1       = "foo.Test1"
		name2       = "foo.Test2"
		preRunMsg   = "setting up for run"
		postRunMsg  = "cleaning up after run"
		preTestMsg  = "setting up for test"
		postTestMsg = "cleaning up for test"
	)

	reg := testing.NewRegistry(testing.NoAutoName)
	reg.AddTest(&testing.Test{
		Name:    name1,
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute},
	)
	reg.AddTest(&testing.Test{
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
	var preRunCalls, postRunCalls, preTestCalls, postTestCalls int
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
		preTestFunc: func(ctx context.Context, s *testing.State) {
			preTestCalls++
			s.Log(preTestMsg)
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

	if _, err := os.Stat(runTmpDir); err == nil {
		t.Errorf("Temporary directory %s still exists", runTmpDir)
	} else if !os.IsNotExist(err) {
		t.Errorf("Failed to stat %s: %v", runTmpDir, err)
	}

	// Just check some basic details of the control messages.
	r := control.NewMessageReader(&stdout)
	for i, ei := range []interface{}{
		&control.RunLog{Text: preRunMsg},
		&control.TestStart{Test: *tests[0]},
		&control.TestLog{Text: preTestMsg},
		&control.TestLog{Text: postTestMsg},
		&control.TestEnd{Name: name1},
		&control.TestStart{Test: *tests[1]},
		&control.TestLog{Text: preTestMsg},
		&control.TestError{},
		&control.TestLog{Text: postTestMsg},
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
	reg := testing.NewRegistry(testing.NoAutoName)

	// The first test blocks indefinitely on a channel.
	const name1 = "foo.Test1"
	ch := make(chan bool, 1)
	defer func() { ch <- true }()
	reg.AddTest(&testing.Test{
		Name:        name1,
		Func:        func(context.Context, *testing.State) { <-ch },
		Timeout:     10 * time.Millisecond,
		ExitTimeout: time.Millisecond, // avoid blocking after timeout
	})

	// The second test blocks for 50 ms and specifies a custom one-minute timeout.
	const name2 = "foo.Test2"
	reg.AddTest(&testing.Test{
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
	if err := runTests(context.Background(), &bytes.Buffer{}, &Args{},
		&runConfig{}, localBundle, []*testing.Test{}); !errorHasStatus(err, statusNoTests) {
		t.Fatalf("runTests() = %v; want status %v", err, statusNoTests)
	}
}

func TestRunTestsMissingDeps(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
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
	testing.AddTest(&testing.Test{Name: validName, Func: makeFunc(validName), SoftwareDeps: []string{validDep}})
	testing.AddTest(&testing.Test{Name: missingName, Func: makeFunc(missingName), SoftwareDeps: []string{missingDep}})
	testing.AddTest(&testing.Test{Name: unregName, Func: makeFunc(unregName), SoftwareDeps: []string{unregDep}})

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

func TestRunMeta(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()

	var meta testing.Meta
	testing.AddTest(&testing.Test{
		Name: "meta.Test",
		Func: func(ctx context.Context, s *testing.State) {
			if m := s.Meta(); m != nil {
				meta = *m
			}
		},
	})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := Args{
		Mode: RunTestsMode,
		RunTests: &RunTestsArgs{
			OutDir:   tmpDir,
			DataDir:  tmpDir,
			TastPath: "/bogus/tast",
			Target:   "root@example.net",
			RunFlags: []string{"-flag1", "-flag2"},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	if status := run(context.Background(), nil, stdin, &bytes.Buffer{}, &bytes.Buffer{}, &Args{},
		&runConfig{defaultTestTimeout: time.Minute}, remoteBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// The test should have access to meta-related information data.
	expMeta := testing.Meta{
		TastPath: args.RunTests.TastPath,
		Target:   args.RunTests.Target,
		RunFlags: args.RunTests.RunFlags,
	}
	if !reflect.DeepEqual(meta, expMeta) {
		t.Errorf("Test got meta %+v; want %+v", meta, expMeta)
	}
}

func TestRunList(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry(testing.NoAutoName))
	defer restore()
	f := func(context.Context, *testing.State) {}
	testing.AddTest(&testing.Test{Name: "pkg.Test", Func: f})
	testing.AddTest(&testing.Test{Name: "pkg.Test2", Func: f})

	var exp bytes.Buffer
	if err := testing.WriteTestsAsJSON(&exp, testing.GlobalRegistry().AllTests()); err != nil {
		t.Fatal(err)
	}

	// ListTestsMode should result in tests being JSON-marshaled to stdout.
	stdin := newBufferWithArgs(t, &Args{Mode: ListTestsMode})
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

func TestPrepareTempDir(t *gotesting.T) {
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

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

	restore()
	restore = nil

	if env := os.Getenv("TMPDIR"); env != origTmpDir {
		t.Errorf("restore did not restore $TMPDIR; got %q, want %q", env, origTmpDir)
	}

	if _, err := os.Stat(tmpDir); err == nil {
		t.Error("restore did not delete the temporary directory")
	} else if !os.IsNotExist(err) {
		t.Error("Stat failed: ", err)
	}
}
