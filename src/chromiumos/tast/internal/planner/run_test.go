// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/devserver/devservertest"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

// runTestsAndReadAll runs tests and returns a slice of control messages written to the output.
func runTestsAndReadAll(t *gotesting.T, tests []*testing.TestInstance, pcfg *Config) []control.Msg {
	t.Helper()

	sink := newOutputSink()
	RunTests(context.Background(), tests, sink, pcfg)
	msgs, err := sink.ReadAll()
	if err != nil {
		t.Fatal("ReadAll: ", err)
	}
	return msgs
}

// testPre implements Precondition for unit tests.
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

func TestRunSuccess(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	od := filepath.Join(td, "out")

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Func:    func(context.Context, *testing.State) {},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{OutDir: od})

	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestEnd{Name: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}

	if fi, err := os.Stat(filepath.Join(od, tests[0].Name)); err != nil {
		t.Errorf("Out dir %v not accessible after testing: %v", od, err)
	} else if mode := fi.Mode()&os.ModePerm | os.ModeSticky; mode != 0777|os.ModeSticky {
		t.Errorf("Out dir %v has mode 0%o; want 0%o", od, mode, 0777|os.ModeSticky)
	}
}

func TestRunPanic(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Func:    func(context.Context, *testing.State) { panic("intentional panic") },
		Timeout: time.Minute,
	}}
	msgs := runTestsAndReadAll(t, tests, &Config{})
	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestError{Error: testing.Error{Reason: "Panic: intentional panic"}},
		&control.TestEnd{Name: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunDeadline(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			// Wait for the context to report that the deadline has been hit.
			<-ctx.Done()
			s.Error("Saw timeout within test")
		},
		Timeout:     time.Millisecond,
		ExitTimeout: 10 * time.Second,
	}}
	msgs := runTestsAndReadAll(t, tests, &Config{})
	// The error that was reported by the test after its deadline was hit
	// but within the exit delay should be available.
	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestError{Error: testing.Error{Reason: "Saw timeout within test"}},
		&control.TestEnd{Name: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunLogAfterTimeout(t *gotesting.T) {
	cont := make(chan bool)
	done := make(chan bool)
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			// Report when we're done, either after completing or after panicking before completion.
			completed := false
			defer func() { done <- completed }()

			// Ignore the deadline and wait until we're told to continue.
			<-ctx.Done()
			<-cont
			s.Log("Done waiting")
			completed = true
		},
		Timeout:     time.Millisecond,
		ExitTimeout: time.Millisecond,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	// Tell the test to continue even though Run has already returned. The output stream should
	// still be writable so as to avoid a panic when the test writes to it (https://crbug.com/853406),
	// but they are dropped.
	cont <- true
	if completed := <-done; !completed {
		t.Error("Test function didn't complete")
	}

	// An error is written with a goroutine dump.
	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestError{Error: testing.Error{Reason: "Test did not return on timeout (see log for goroutine dump)"}},
		&control.TestLog{Text: "Dumping all goroutines"},
		// A goroutine dump follows. Do not compare them as the content is undeterministic.
	}
	if diff := cmp.Diff(msgs[:len(want)], want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunLateWriteFromGoroutine(t *gotesting.T) {
	// Run a test that calls s.Log from a goroutine after the test has finished.
	start := make(chan struct{}) // tells goroutine to start
	end := make(chan struct{})   // announces goroutine is done
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			go func() {
				<-start
				s.Log("This message should be still reported")
				close(end)
			}()
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	// Tell the goroutine to start and wait for it to finish.
	close(start)
	<-end

	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		// Log message from the goroutine is not reported.
		&control.TestEnd{Name: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunSkipStages(t *gotesting.T) {
	// action specifies an action performed in a stage.
	type action int
	const (
		pass    action = iota
		doError        // call State.Error
		doFatal        // call State.Fatal
		doPanic        // call panic()
		noCall         // stage should be skipped
	)

	// testBehavior specifies the behavior of a test in each of its stages.
	type testBehavior struct {
		pre                *testPre
		preTestAction      action // TestConfig.PreTestFunc
		prepareAction      action // Precondition.Prepare
		testAction         action // Test.Func
		closeAction        action // Precondition.Close
		postTestAction     action // TestConfig.PostTestFunc
		postTestHookAction action // Return of TestConfig.PreTestFunc
	}

	// pre1 and pre2 are preconditions used in tests. prepareFunc and closeFunc
	// are filled in each subtest.
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}

	for _, tc := range []struct {
		name  string
		tests []testBehavior
		want  []control.Msg
	}{
		{
			name: "no precondition",
			tests: []testBehavior{
				{nil, pass, noCall, pass, noCall, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
			},
		},
		{
			name: "passes",
			tests: []testBehavior{
				{pre1, pass, pass, pass, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestLog{Text: "prepare: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
			},
		},
		{
			name: "pretest fails",
			tests: []testBehavior{
				{pre1, doError, noCall, noCall, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestError{Error: testing.Error{Reason: "preTest: Intentional error"}},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
			},
		},
		{
			name: "pretest panics",
			tests: []testBehavior{
				{pre1, doPanic, noCall, noCall, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestError{Error: testing.Error{Reason: "Panic: preTest: Intentional panic"}},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestEnd{Name: "0"},
			},
		},
		{
			name: "prepare fails",
			tests: []testBehavior{
				{pre1, pass, doError, noCall, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestError{Error: testing.Error{Reason: "[Precondition failure] prepare: Intentional error"}},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
			},
		},
		{
			name: "prepare panics",
			tests: []testBehavior{
				{pre1, pass, doPanic, noCall, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestError{Error: testing.Error{Reason: "[Precondition failure] Panic: prepare: Intentional panic"}},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
			},
		},
		{
			name: "same precondition",
			tests: []testBehavior{
				{pre1, pass, pass, pass, noCall, pass, pass},
				{pre1, pass, pass, pass, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestLog{Text: "prepare: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
				&control.TestStart{Test: testing.TestInfo{Name: "1", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestLog{Text: "prepare: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "1"},
			},
		},
		{
			name: "different preconditions",
			tests: []testBehavior{
				{pre1, pass, pass, pass, pass, pass, pass},
				{pre2, pass, pass, pass, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestLog{Text: "prepare: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
				&control.TestStart{Test: testing.TestInfo{Name: "1", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre2"`},
				&control.TestLog{Text: "prepare: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: `Closing precondition "pre2"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "1"},
			},
		},
		{
			name: "first prepare fails",
			tests: []testBehavior{
				{pre1, pass, doError, noCall, noCall, pass, pass},
				{pre1, pass, pass, pass, pass, pass, pass},
			},
			want: []control.Msg{
				&control.TestStart{Test: testing.TestInfo{Name: "0", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestError{Error: testing.Error{Reason: "[Precondition failure] prepare: Intentional error"}},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "0"},
				&control.TestStart{Test: testing.TestInfo{Name: "1", Timeout: time.Minute}},
				&control.TestLog{Text: "preTest: OK"},
				&control.TestLog{Text: `Preparing precondition "pre1"`},
				&control.TestLog{Text: "prepare: OK"},
				&control.TestLog{Text: "test: OK"},
				&control.TestLog{Text: `Closing precondition "pre1"`},
				&control.TestLog{Text: "close: OK"},
				&control.TestLog{Text: "postTest: OK"},
				&control.TestLog{Text: "postTestHook: OK"},
				&control.TestEnd{Name: "1"},
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			type state interface {
				Logf(fmt string, args ...interface{})
				Errorf(fmt string, args ...interface{})
				Fatalf(fmt string, args ...interface{})
			}
			doAction := func(s state, a action, stage string) {
				switch a {
				case pass:
					s.Logf("%s: OK", stage)
				case doError:
					s.Errorf("%s: Intentional error", stage)
				case doFatal:
					s.Fatalf("%s: Intentional fatal", stage)
				case doPanic:
					panic(fmt.Sprintf("%s: Intentional panic", stage))
				case noCall:
					t.Errorf("%s: Called unexpectedly", stage)
				}
			}

			currentBehavior := func(s interface{ OutDir() string }) testBehavior {
				// Abuse OutDir to tell which test we're running currently.
				i, err := strconv.Atoi(filepath.Base(s.OutDir()))
				if err != nil {
					t.Fatal("Failed to parse OutDir: ", err)
				}
				return tc.tests[i]
			}

			var tests []*testing.TestInstance
			for i, tb := range tc.tests {
				test := &testing.TestInstance{
					Name: strconv.Itoa(i),
					Func: func(ctx context.Context, s *testing.State) {
						doAction(s, currentBehavior(s).testAction, "test")
					},
					Timeout: time.Minute,
				}
				// We can't just do "test.Pre =tbc.pre" here. See e.g. https://tour.golang.org/methods/12:
				// "Note that an interface value that holds a nil concrete value is itself non-nil."
				if tb.pre != nil {
					test.Pre = tb.pre
				}
				tests = append(tests, test)
			}

			for _, pre := range []*testPre{pre1, pre2} {
				pre.prepareFunc = func(ctx context.Context, s *testing.PreState) interface{} {
					doAction(s, currentBehavior(s).prepareAction, "prepare")
					return nil
				}
				pre.closeFunc = func(ctx context.Context, s *testing.PreState) {
					doAction(s, currentBehavior(s).closeAction, "close")
				}
			}

			outDir := testutil.TempDir(t)
			defer os.RemoveAll(outDir)

			pcfg := &Config{
				OutDir: outDir,
				PreTestFunc: func(ctx context.Context, s *testing.State) func(context.Context, *testing.State) {
					doAction(s, currentBehavior(s).preTestAction, "preTest")
					return func(ctx context.Context, s *testing.State) {
						doAction(s, currentBehavior(s).postTestHookAction, "postTestHook")
					}
				},
				PostTestFunc: func(ctx context.Context, s *testing.State) {
					doAction(s, currentBehavior(s).postTestAction, "postTest")
				},
			}
			msgs := runTestsAndReadAll(t, tests, pcfg)
			if diff := cmp.Diff(msgs, tc.want); diff != "" {
				t.Error("Output mismatch (-got +want):\n", diff)
			}
		})
	}
}

func TestRunExternalData(t *gotesting.T) {
	const (
		file1URL  = "gs://bucket/file1.txt"
		file1Path = "pkg/data/file1.txt"
		file1Data = "data1"
		file2URL  = "gs://bucket/file2.txt"
		file2Path = "pkg/data/file2.txt"
		file2Data = "data2"
		file3URL  = "gs://bucket/file3.txt"
		file3Path = "pkg/data/file3.txt"
	)

	ds, err := devservertest.NewServer(devservertest.Files([]*devservertest.File{
		{URL: file1URL, Data: []byte(file1Data)},
		{URL: file2URL, Data: []byte(file2Data)},
		// file3.txt is missing.
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()

	tests := []*testing.TestInstance{
		{
			Name:         "example.Test1",
			Pkg:          "pkg",
			Func:         func(ctx context.Context, s *testing.State) {},
			Data:         []string{"file1.txt"},
			SoftwareDeps: []string{"dep1"},
			Timeout:      time.Minute,
		},
		{
			Name:         "example.Test2",
			Pkg:          "pkg",
			Func:         func(ctx context.Context, s *testing.State) {},
			Data:         []string{"file2.txt"},
			SoftwareDeps: []string{"dep2"},
			Timeout:      time.Minute,
		},
		{
			Name:    "example.Test3",
			Pkg:     "pkg",
			Func:    func(ctx context.Context, s *testing.State) {},
			Data:    []string{"file3.txt"},
			Timeout: time.Minute,
		},
	}

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
		file3Path + testing.ExternalLinkSuffix: buildLink(file3URL, ""),
	}); err != nil {
		t.Fatal("WriteFiles: ", err)
	}

	pcfg := &Config{
		DataDir: dataDir,
		Features: dep.Features{
			Software: &dep.SoftwareFeatures{
				Available:   []string{"dep2"},
				Unavailable: []string{"dep1"},
			},
		},
		Devservers: []string{ds.URL},
	}

	msgs := runTestsAndReadAll(t, tests, pcfg)

	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestEnd{Name: tests[0].Name, SkipReasons: []string{"missing SoftwareDeps: dep1"}},
		&control.TestStart{Test: *tests[1].TestInfo()},
		&control.TestEnd{Name: tests[1].Name},
		&control.TestStart{Test: *tests[2].TestInfo()},
		&control.TestError{Error: testing.Error{Reason: "Required data file file3.txt missing: failed to download gs://bucket/file3.txt: file does not exist"}},
		&control.TestEnd{Name: tests[2].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}

	files, err := testutil.ReadFiles(dataDir)
	if err != nil {
		t.Fatal("ReadFiles: ", err)
	}
	wantFiles := map[string]string{
		// file1.txt is not downloaded since pkg.Test1 is not run.
		file1Path + testing.ExternalLinkSuffix:  buildLink(file1URL, file1Data),
		file2Path:                               file2Data,
		file2Path + testing.ExternalLinkSuffix:  buildLink(file2URL, file2Data),
		file3Path + testing.ExternalLinkSuffix:  buildLink(file3URL, ""),
		file3Path + testing.ExternalErrorSuffix: "failed to download gs://bucket/file3.txt: file does not exist",
	}
	if diff := cmp.Diff(files, wantFiles); diff != "" {
		t.Error("Data directory mismatch (-got +want):\n", diff)
	}
}

func TestRunPrecondition(t *gotesting.T) {
	type data struct{}
	preData := &data{}

	// The test should be able to access the data via State.PreValue.
	tests := []*testing.TestInstance{{
		// Use a precondition that returns the struct that we declared earlier from its Prepare method.
		Pre: &testPre{
			name:        "pre",
			prepareFunc: func(context.Context, *testing.PreState) interface{} { return preData },
		},
		Func: func(ctx context.Context, s *testing.State) {
			if s.PreValue() == nil {
				s.Fatal("Precondition value not supplied")
			} else if d, ok := s.PreValue().(*data); !ok {
				s.Fatal("Precondition value didn't have expected type")
			} else if d != preData {
				s.Fatalf("Got precondition value %v; want %v", d, preData)
			}
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestLog{Text: `Preparing precondition "pre"`},
		&control.TestLog{Text: `Closing precondition "pre"`},
		&control.TestEnd{Name: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunPreconditionContext(t *gotesting.T) {
	var prevCtx context.Context

	prepareFunc := func(ctx context.Context, s *testing.PreState) interface{} {
		pctx := s.PreCtx()
		if prevCtx == nil {
			// This is the first test. Save pctx.
			prevCtx = pctx
		} else {
			// This is the second test. Ensure prevCtx is still alive.
			if err := prevCtx.Err(); err != nil {
				t.Error("Prepare (second test): PreCtx was canceled: ", err)
			}
		}

		if _, ok := testing.ContextSoftwareDeps(pctx); !ok {
			t.Error("ContextSoftwareDeps unavailable")
		}
		return nil
	}

	closeFunc := func(ctx context.Context, s *testing.PreState) {
		if prevCtx != nil {
			if err := prevCtx.Err(); err != nil {
				t.Error("Close: PreCtx was canceled: ", err)
			}
		}
	}

	testFunc := func(ctx context.Context, s *testing.State) {}

	pre := &testPre{
		name:        "pre",
		prepareFunc: prepareFunc,
		closeFunc:   closeFunc,
	}

	tests := []*testing.TestInstance{
		{Name: "t1", Pre: pre, Timeout: time.Minute, Func: testFunc},
		{Name: "t2", Pre: pre, Timeout: time.Minute, Func: testFunc},
	}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestLog{Text: `Preparing precondition "pre"`},
		&control.TestEnd{Name: tests[0].Name},
		&control.TestStart{Test: *tests[1].TestInfo()},
		&control.TestLog{Text: `Preparing precondition "pre"`},
		&control.TestLog{Text: `Closing precondition "pre"`},
		&control.TestEnd{Name: tests[1].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
	if prevCtx.Err() == nil {
		t.Error("Context not cancelled")
	}
}

type runPhase int

const (
	phasePreTestFunc runPhase = iota
	phasePrepareFunc
	phaseTestFunc
	phaseSubtestFunc
	phaseCloseFunc
	phasePostTestFunc
	phasePostTestHook
)

func (p runPhase) String() string {
	switch p {
	case phasePreTestFunc:
		return "preTestFunc"
	case phasePrepareFunc:
		return "prepareFunc"
	case phaseTestFunc:
		return "testFunc"
	case phaseSubtestFunc:
		return "subtestFunc"
	case phaseCloseFunc:
		return "closeFunc"
	case phasePostTestFunc:
		return "postTestFunc"
	case phasePostTestHook:
		return "postTestHook"
	default:
		return "unknown"
	}
}

func TestRunHasError(t *gotesting.T) {
	type stateLike interface {
		Error(args ...interface{})
		HasError() bool
	}

	for _, failIn := range []runPhase{
		phasePreTestFunc,
		phasePrepareFunc,
		phaseTestFunc,
		phaseSubtestFunc,
		phaseCloseFunc,
		phasePostTestFunc,
		phasePostTestHook,
	} {
		t.Run(fmt.Sprintf("Fail in %s", failIn), func(t *gotesting.T) {
			onPhase := func(s stateLike, current runPhase) {
				got := s.HasError()
				want := current > failIn
				if got != want {
					t.Errorf("Phase %v: HasError()=%t; want %t", current, got, want)
				}
				if current == failIn {
					s.Error("Failure")
				}
			}

			pre := &testPre{
				prepareFunc: func(ctx context.Context, s *testing.PreState) interface{} {
					onPhase(s, phasePrepareFunc)
					return nil
				},
				closeFunc: func(ctx context.Context, s *testing.PreState) {
					onPhase(s, phaseCloseFunc)
				},
			}
			pcfg := &Config{
				PreTestFunc: func(ctx context.Context, s *testing.State) func(context.Context, *testing.State) {
					onPhase(s, phasePreTestFunc)
					return func(ctx context.Context, s *testing.State) {
						onPhase(s, phasePostTestHook)
					}
				},
				PostTestFunc: func(ctx context.Context, s *testing.State) {
					onPhase(s, phasePostTestFunc)
				},
			}
			testFunc := func(ctx context.Context, s *testing.State) {
				onPhase(s, phaseTestFunc)
				s.Run(ctx, "subtest", func(ctx context.Context, s *testing.State) {
					// Subtests are somewhat special; they do not inherit the error status from the parent state.
					if s.HasError() {
						t.Errorf("Phase %v: HasError()=true; want false", phaseSubtestFunc)
					}
					if failIn == phaseSubtestFunc {
						s.Error("Failure")
					}
				})
			}
			tests := []*testing.TestInstance{{Name: "t", Pre: pre, Timeout: time.Minute, Func: testFunc}}

			runTestsAndReadAll(t, tests, pcfg)
		})
	}
}

func TestAttachStateToContext(t *gotesting.T) {
	tests := []*testing.TestInstance{{
		Func: func(ctx context.Context, s *testing.State) {
			logging.ContextLog(ctx, "msg ", 1)
			logging.ContextLogf(ctx, "msg %d", 2)
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []control.Msg{
		&control.TestStart{Test: *tests[0].TestInfo()},
		&control.TestLog{Text: "msg 1"},
		&control.TestLog{Text: "msg 2"},
		&control.TestEnd{Name: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}
