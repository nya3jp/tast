// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/devserver/devservertest"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/jsonprotocol"
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

// errorHasStatus returns true if err is of type *command.StatusError and contains the supplied status code.
func errorHasStatus(err error, status int) bool {
	if se, ok := err.(*command.StatusError); !ok {
		return false
	} else if se.Status() != status {
		return false
	}
	return true
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
	tests := reg.AllTests()
	var preRunCalls, postRunCalls, preTestCalls, postTestCalls int
	args := BundleArgs{
		RunTests: &BundleRunTestsArgs{
			OutDir:  tmpDir,
			DataDir: tmpDir,
			TempDir: runTmpDir,
		},
	}
	scfg := staticConfig{
		runHook: func(ctx context.Context) (func(context.Context) error, error) {
			preRunCalls++
			testcontext.Log(ctx, preRunMsg)
			return func(ctx context.Context) error {
				postRunCalls++
				testcontext.Log(ctx, postRunMsg)
				return nil
			}, nil
		},
		testHook: func(ctx context.Context, s *testing.TestHookState) func(ctx context.Context, s *testing.TestHookState) {
			preTestCalls++
			s.Log(preTestMsg)

			return func(ctx context.Context, s *testing.TestHookState) {
				postTestCalls++
				s.Log(postTestMsg)
			}
		},
	}

	sig := fmt.Sprintf("runTests(..., %+v, %+v)", args, scfg)
	if err := runTests(context.Background(), &stdout, &args, &scfg, localBundle, tests); err != nil {
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

func TestRunTestsTimeout(t *gotesting.T) {
	reg := testing.NewRegistry()

	// The first test blocks indefinitely on a channel.
	const name1 = "foo.Test1"
	ch := make(chan bool, 1)
	defer func() { ch <- true }()
	reg.AddTestInstance(&testing.TestInstance{
		Name:        name1,
		Func:        func(context.Context, *testing.State) { <-ch },
		Timeout:     time.Millisecond,
		ExitTimeout: time.Millisecond, // avoid blocking after timeout
	})

	// The second test passes.
	const name2 = "foo.Test2"
	reg.AddTestInstance(&testing.TestInstance{
		Name:    name2,
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute,
	})

	stdout := bytes.Buffer{}
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	args := BundleArgs{
		RunTests: &BundleRunTestsArgs{
			OutDir:  tmpDir,
			DataDir: tmpDir,
		},
	}

	// The first test should time out after 1 millisecond.
	// The second test is not run.
	if err := runTests(context.Background(), &stdout, &args, &staticConfig{}, localBundle, reg.AllTests()); err == nil {
		t.Fatalf("runTests(..., %+v, ...) succeeded unexpectedly", args)
	}

	// EntityStart, EntityError and EntityEnd should be observed exactly once for the first test.
	seenStart := 0
	seenError := 0
	seenEnd := 0
	foundMe := false
	r := control.NewMessageReader(&stdout)
	for r.More() {
		if msg, err := r.ReadMessage(); err != nil {
			t.Error("ReadMessage failed: ", err)
		} else if ts, ok := msg.(*control.EntityStart); ok {
			if ts.Info.Name != name1 {
				t.Errorf("EntityStart.Test.Name = %q; want %q", ts.Info.Name, name1)
			}
			seenStart++
		} else if tl, ok := msg.(*control.EntityLog); ok {
			// The log should contain stack traces, including this test function.
			if strings.Contains(tl.Text, "TestRunTestsTimeout") {
				foundMe = true
			}
		} else if _, ok := msg.(*control.EntityError); ok {
			seenError++
		} else if te, ok := msg.(*control.EntityEnd); ok {
			if te.Name != name1 {
				t.Errorf("EntityEnd.Name = %q; want %q", te.Name, name1)
			}
			seenEnd++
		}
	}
	if seenStart != 1 {
		t.Errorf("Got EntityStart %d time(s); want 1 time", seenStart)
	}
	if seenError != 1 {
		t.Errorf("Got EntityError %d time(s); want 1 time", seenError)
	}
	if seenEnd != 1 {
		t.Errorf("Got EntityEnd %d time(s); want 1 time", seenEnd)
	}
	if !foundMe {
		t.Error("Stack trace not found")
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	// runTests should report failure when passed an empty slice of tests.
	if err := runTests(context.Background(), &bytes.Buffer{}, &BundleArgs{RunTests: &BundleRunTestsArgs{}},
		&staticConfig{}, localBundle, []*testing.TestInstance{}); !errorHasStatus(err, statusNoTests) {
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

	// Register four tests: one with a satisfied dep, another with a missing SW dep,
	// another with a missing var dep, and a fourth with an unregistered dep.
	testRan := make(map[string]bool)
	makeFunc := func(name string) testing.TestFunc {
		return func(context.Context, *testing.State) { testRan[name] = true }
	}
	testing.AddTestInstance(&testing.TestInstance{Name: validName, Func: makeFunc(validName), SoftwareDeps: []string{validDep}})
	testing.AddTestInstance(&testing.TestInstance{Name: missingName, Func: makeFunc(missingName), SoftwareDeps: []string{missingDep}})
	testing.AddTestInstance(&testing.TestInstance{Name: unregName, Func: makeFunc(unregName), SoftwareDeps: []string{unregDep}})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  tmpDir,
			DataDir: tmpDir,
			FeatureArgs: FeatureArgs{
				CheckSoftwareDeps:           true,
				TestVars:                    map[string]string{},
				AvailableSoftwareFeatures:   []string{validDep},
				UnavailableSoftwareFeatures: []string{missingDep},
			},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, &BundleArgs{},
		&staticConfig{defaultTestTimeout: time.Minute}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// Read through the control messages to get test results.
	var testName string
	testFailed := make(map[string][]jsonprotocol.Error)
	testSkipped := make(map[string]bool)
	r := control.NewMessageReader(stdout)
	for r.More() {
		msg, err := r.ReadMessage()
		if err != nil {
			t.Fatal("Failed to read message:", err)
		}
		switch m := msg.(type) {
		case *control.EntityStart:
			testName = m.Info.Name
		case *control.EntityEnd:
			testSkipped[testName] = len(m.SkipReasons) > 0 || len(m.DeprecatedMissingSoftwareDeps) > 0
		case *control.EntityError:
			testFailed[testName] = append(testFailed[testName], m.Error)
		}
	}

	// Verify that the expected results were reported for each test.
	for _, tc := range []struct {
		name       string
		shouldRun  bool
		shouldFail bool
		shouldSkip bool
	}{
		{validName, true, false, false},
		{missingName, false, false, true},
		{unregName, false, true, false},
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
		if skipped := testSkipped[tc.name]; skipped && !tc.shouldSkip {
			t.Errorf("%v skipped unexpectedly", tc.name)
		} else if !skipped && tc.shouldSkip {
			t.Errorf("%v didn't skip", tc.name)
		}
	}
}

func TestRunTestsSkipTestWithPrecondition(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	var actions []string
	makePre := func(name string) *testPre {
		return &testPre{
			prepareFunc: func(context.Context, *testing.PreState) interface{} {
				actions = append(actions, "prepare_"+name)
				return nil
			},
			closeFunc: func(context.Context, *testing.PreState) { actions = append(actions, "close_"+name) },
			name:      name,
		}
	}
	pre1 := makePre("pre1")
	pre2 := makePre("pre2")

	// Make the last test using each precondition get skipped due to unsatisfied dependencies.
	f := func(context.Context, *testing.State) {}
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test1", Func: f, Pre: pre1})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test2", Func: f, Pre: pre1})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test3", Func: f, Pre: pre1, SoftwareDeps: []string{"dep"}})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test4", Func: f, Pre: pre2})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test5", Func: f, Pre: pre2})
	testing.AddTestInstance(&testing.TestInstance{Name: "pkg.Test6", Func: f, Pre: pre2, SoftwareDeps: []string{"dep"}})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  tmpDir,
			DataDir: tmpDir,
			FeatureArgs: FeatureArgs{
				CheckSoftwareDeps:           true,
				UnavailableSoftwareFeatures: []string{"dep"},
			},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, &BundleArgs{},
		&staticConfig{defaultTestTimeout: time.Minute}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// We should've still closed each precondition after running the last test that needs it: https://crbug.com/950499
	exp := []string{"prepare_pre1", "prepare_pre1", "close_pre1", "prepare_pre2", "prepare_pre2", "close_pre2"}
	if !reflect.DeepEqual(actions, exp) {
		t.Errorf("run() performed actions %v; want %v", actions, exp)
	}
}

func TestRunRemoteData(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	var (
		meta *testing.Meta
		hint *testing.RPCHint
		dt   *dut.DUT
	)
	testing.AddTestInstance(&testing.TestInstance{
		Name: "meta.Test",
		Func: func(ctx context.Context, s *testing.State) {
			meta = s.Meta()
			hint = s.RPCHint()
			dt = s.DUT()
		},
	})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:         tmpDir,
			DataDir:        tmpDir,
			TastPath:       "/bogus/tast",
			Target:         td.Srvs[0].Addr().String(),
			KeyFile:        td.UserKeyFile,
			RunFlags:       []string{"-flag1", "-flag2"},
			LocalBundleDir: "/mock/local/bundles",
			FeatureArgs: FeatureArgs{
				TestVars: map[string]string{"var1": "value1"},
			},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	if status := run(context.Background(), nil, stdin, &bytes.Buffer{}, &bytes.Buffer{}, &BundleArgs{},
		&staticConfig{defaultTestTimeout: time.Minute}, remoteBundle); status != statusSuccess {
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

func TestRunCloudStorage(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	testing.AddTestInstance(&testing.TestInstance{
		Name: "example.Test",
		Func: func(ctx context.Context, s *testing.State) {
			if s.CloudStorage() == nil {
				t.Error("testing.State.CloudStorage is nil")
			}
		},
	})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:         tmpDir,
			DataDir:        tmpDir,
			TastPath:       "/bogus/tast",
			Target:         td.Srvs[0].Addr().String(),
			KeyFile:        td.UserKeyFile,
			RunFlags:       []string{"-flag1", "-flag2"},
			LocalBundleDir: "/mock/local/bundles",
		},
	}
	stdin := newBufferWithArgs(t, &args)
	if status := run(context.Background(), nil, stdin, &bytes.Buffer{}, &bytes.Buffer{}, &BundleArgs{},
		&staticConfig{defaultTestTimeout: time.Minute}, remoteBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}
}

func TestRunExternalDataFiles(t *gotesting.T) {
	const (
		file1URL  = "gs://bucket/file1.txt"
		file1Path = "pkg/data/file1.txt"
		file1Data = "data1"
		file2URL  = "gs://bucket/file2.txt"
		file2Path = "pkg/data/file2.txt"
		file2Data = "data2"
	)

	td := sshtest.NewTestData(nil)
	defer td.Close()

	ds, err := devservertest.NewServer(devservertest.Files([]*devservertest.File{
		{URL: file1URL, Data: []byte(file1Data)},
		{URL: file2URL, Data: []byte(file2Data)},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	testing.AddTestInstance(&testing.TestInstance{
		Name:         "example.Test1",
		Pkg:          "pkg",
		Func:         func(ctx context.Context, s *testing.State) {},
		Data:         []string{"file1.txt"},
		SoftwareDeps: []string{"dep1"},
	})
	testing.AddTestInstance(&testing.TestInstance{
		Name:         "example.Test2",
		Pkg:          "pkg",
		Func:         func(ctx context.Context, s *testing.State) {},
		Data:         []string{"file2.txt"},
		SoftwareDeps: []string{"dep2"},
	})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	buildLink := func(url, data string) string {
		hash := sha256.Sum256([]byte(data))
		ld := &extdata.LinkData{
			Type:      extdata.TypeStatic,
			StaticURL: url,
			Size:      int64(len(data)),
			SHA256Sum: hex.EncodeToString(hash[:]),
		}
		b, err := json.Marshal(ld)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	dataDir := filepath.Join(tmpDir, "data")
	if err := testutil.WriteFiles(dataDir, map[string]string{
		file1Path + testing.ExternalLinkSuffix: buildLink(file1URL, file1Data),
		file2Path + testing.ExternalLinkSuffix: buildLink(file2URL, file2Data),
	}); err != nil {
		t.Fatal("WriteFiles: ", err)
	}

	args := BundleArgs{
		Mode: BundleRunTestsMode,
		RunTests: &BundleRunTestsArgs{
			OutDir:  filepath.Join(tmpDir, "out"),
			DataDir: dataDir,
			Target:  td.Srvs[0].Addr().String(),
			KeyFile: td.UserKeyFile,
			FeatureArgs: FeatureArgs{
				CheckSoftwareDeps:           true,
				AvailableSoftwareFeatures:   []string{"dep1"},
				UnavailableSoftwareFeatures: []string{"dep2"},
			},
			Devservers: []string{ds.URL},
		},
	}
	stdin := newBufferWithArgs(t, &args)
	if status := run(context.Background(), nil, stdin, ioutil.Discard, ioutil.Discard, &BundleArgs{},
		&staticConfig{defaultTestTimeout: time.Minute}, remoteBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}

	// file1.txt is downloaded, but file2.txt is not due to missing software dependencies.
	files, err := testutil.ReadFiles(dataDir)
	if err != nil {
		t.Fatal("ReadFiles: ", err)
	}
	exp := map[string]string{
		file1Path:                              file1Data,
		file1Path + testing.ExternalLinkSuffix: buildLink(file1URL, file1Data),
		file2Path + testing.ExternalLinkSuffix: buildLink(file2URL, file2Data),
	}
	if diff := cmp.Diff(files, exp); diff != "" {
		t.Error("Unexpected data files after run (-got +want):\n", diff)
	}
}

func TestRunStartFixture(t *gotesting.T) {
	// runTests should not run runHook if tests depend on remote fixtures.
	// TODO(crbug/1184567): consider long term plan about interactions between
	// remote fixtures and run hooks.
	if err := runTests(context.Background(), &bytes.Buffer{}, &BundleArgs{RunTests: &BundleRunTestsArgs{
		StartFixtureName: "foo",
	}}, &staticConfig{
		runHook: func(context.Context) (func(context.Context) error, error) {
			t.Error("runHook unexpectedly called")
			return nil, nil
		},
	}, localBundle, []*testing.TestInstance{{
		Fixture: "foo",
		Name:    "pkg.Test",
		Func:    func(context.Context, *testing.State) {},
	}}); err != nil {
		t.Fatalf("runTests(): %v", err)
	}

	// If StartFixtureName is empty, runHook should run.
	called := false
	if err := runTests(context.Background(), &bytes.Buffer{}, &BundleArgs{RunTests: &BundleRunTestsArgs{
		StartFixtureName: "",
	}}, &staticConfig{
		runHook: func(context.Context) (func(context.Context) error, error) {
			called = true
			return nil, nil
		},
	}, localBundle, []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(context.Context, *testing.State) {},
	}}); err != nil {
		t.Fatalf("runTests(): %v", err)
	}
	if !called {
		t.Error("runHook was not called")
	}
}

func TestRunList(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	f := func(context.Context, *testing.State) {}
	tests := []*testing.TestInstance{
		{Name: "pkg.Test", Func: f},
		{Name: "pkg.Test2", Func: f},
	}

	for _, test := range tests {
		testing.AddTestInstance(test)
	}

	var infos []*jsonprotocol.EntityWithRunnabilityInfo
	for _, test := range tests {
		infos = append(infos, &jsonprotocol.EntityWithRunnabilityInfo{
			EntityInfo: *jsonprotocol.MustEntityInfoFromProto(test.EntityProto()),
		})
	}

	var exp bytes.Buffer
	if err := json.NewEncoder(&exp).Encode(infos); err != nil {
		t.Fatal(err)
	}

	// BundleListTestsMode should result in tests being JSON-marshaled to stdout.
	stdin := newBufferWithArgs(t, &BundleArgs{Mode: BundleListTestsMode, ListTests: &BundleListTestsArgs{}})
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{},
		&BundleArgs{}, &staticConfig{}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run() wrote %q; want %q", stdout.String(), exp.String())
	}

	// The -dumptests command-line flag should do the same thing.
	clArgs := []string{"-dumptests"}
	stdout.Reset()
	if status := run(context.Background(), clArgs, &bytes.Buffer{}, stdout, &bytes.Buffer{},
		&BundleArgs{}, &staticConfig{}, localBundle); status != statusSuccess {
		t.Fatalf("run(%v) returned status %v; want %v", clArgs, status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run(%v) wrote %q; want %q", clArgs, stdout.String(), exp.String())
	}
}

// TestRunListWithDep tests run.run for listing test with dependency check.
func TestRunListWithDep(t *gotesting.T) {
	const (
		validDep   = "valid"
		missingDep = "missing"
	)

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	f := func(context.Context, *testing.State) {}
	tests := []*testing.TestInstance{
		{Name: "pkg.Test", Func: f, SoftwareDeps: []string{validDep}},
		{Name: "pkg.Test2", Func: f, SoftwareDeps: []string{missingDep}},
	}

	expectedPassTests := map[string]struct{}{tests[0].Name: struct{}{}}
	expectedSkipTests := map[string]struct{}{tests[1].Name: struct{}{}}

	for _, test := range tests {
		testing.AddTestInstance(test)
	}

	args := BundleArgs{
		Mode: BundleListTestsMode,
		ListTests: &BundleListTestsArgs{
			FeatureArgs: FeatureArgs{
				CheckSoftwareDeps:           true,
				TestVars:                    map[string]string{},
				AvailableSoftwareFeatures:   []string{validDep},
				UnavailableSoftwareFeatures: []string{missingDep},
			},
		},
	}

	// BundleListTestsMode should result in tests being JSON-marshaled to stdout.
	stdin := newBufferWithArgs(t, &args)
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{},
		&BundleArgs{}, &staticConfig{}, localBundle); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}
	var ts []jsonprotocol.EntityWithRunnabilityInfo
	if err := json.Unmarshal(stdout.Bytes(), &ts); err != nil {
		t.Fatalf("unmarshal output %q: %v", stdout.String(), err)
	}
	if len(ts) != len(tests) {
		t.Fatalf("run() returned %v entities; want %v", len(ts), len(tests))
	}
	for _, test := range ts {
		if _, ok := expectedPassTests[test.Name]; ok {
			if test.SkipReason != "" {
				t.Fatalf("run() returned test %q with skip reason %q; want none", test.Name, test.SkipReason)
			}
		}
		if _, ok := expectedSkipTests[test.Name]; ok {
			if test.SkipReason == "" {
				t.Fatalf("run() returned test %q with no skip reason; want %q", test.Name, test.SkipReason)
			}
		}
	}
}

func TestRunListFixtures(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	fixts := []*testing.Fixture{
		{Name: "b", Parent: "a"},
		{Name: "c"},
		{Name: "d"},
		{Name: "a"},
	}

	for _, f := range fixts {
		testing.AddFixture(f)
	}

	// BundleListFixturesMode should output JSON-marshaled fixtures to stdout.
	stdin := newBufferWithArgs(t, &BundleArgs{Mode: BundleListFixturesMode})
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{},
		&BundleArgs{}, &staticConfig{}, localBundle); status != statusSuccess {
		t.Fatalf("run() = %v, want %v", status, statusSuccess)
	}

	got := make([]*jsonprotocol.EntityInfo, 0)
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", stdout.String(), err)
	}
	bundle := filepath.Base(os.Args[0])
	want := []*jsonprotocol.EntityInfo{
		{Type: jsonprotocol.EntityFixture, Name: "a", Bundle: bundle},
		{Type: jsonprotocol.EntityFixture, Name: "b", Fixture: "a", Bundle: bundle},
		{Type: jsonprotocol.EntityFixture, Name: "c", Bundle: bundle},
		{Type: jsonprotocol.EntityFixture, Name: "d", Bundle: bundle},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Output mismatch (-got +want): %v", diff)
	}
}

func TestRunRegistrationError(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	const name = "cat.MyTest"
	testing.AddTestInstance(&testing.TestInstance{Name: name, Func: testFunc})

	// Adding a test with same name should generate an error.
	testing.AddTestInstance(&testing.TestInstance{Name: name, Func: testFunc})

	stdin := newBufferWithArgs(t, &BundleArgs{Mode: BundleListTestsMode, ListTests: &BundleListTestsArgs{}})
	if status := run(context.Background(), nil, stdin, ioutil.Discard, ioutil.Discard,
		&BundleArgs{}, &staticConfig{}, localBundle); status != statusBadTests {
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
	testing.AddTestInstance(&testing.TestInstance{Name: test2, Func: testFunc})
	testing.AddTestInstance(&testing.TestInstance{Name: test3, Func: testFunc})
	testing.AddTestInstance(&testing.TestInstance{Name: test1, Func: testFunc})

	tests, err := testsToRun(&staticConfig{}, nil)
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
	testing.AddTestInstance(&testing.TestInstance{Name: name1, Func: testFunc, Timeout: customTimeout})
	testing.AddTestInstance(&testing.TestInstance{Name: name2, Func: testFunc})

	tests, err := testsToRun(&staticConfig{defaultTestTimeout: defaultTimeout}, nil)
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
