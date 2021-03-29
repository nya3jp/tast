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

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/devserver/devservertest"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

// runTestsAndReadAll runs tests and returns a slice of control messages written to the output.
func runTestsAndReadAll(t *gotesting.T, tests []*testing.TestInstance, pcfg *Config) []proto.Message {
	t.Helper()

	sink := newOutputSink()
	if err := RunTests(context.Background(), tests, sink, pcfg); err != nil {
		t.Logf("RunTests: %v", err) // improve debuggability
	}
	return sink.ReadAll()
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

	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto(), OutDir: filepath.Join(od, "pkg.Test")},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
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
	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "Panic: intentional panic"}},
		&protocol.EntityEndEvent{EntityName: "pkg.Test"},
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
		Timeout: time.Millisecond,
	}}
	gracePeriod := 10 * time.Second
	msgs := runTestsAndReadAll(t, tests, &Config{CustomGracePeriod: &gracePeriod})
	// The error that was reported by the test after its deadline was hit
	// but within the exit delay should be available.
	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "Saw timeout within test"}},
		&protocol.EntityEndEvent{EntityName: "pkg.Test"},
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
		Timeout: time.Millisecond,
	}}

	gracePeriod := time.Millisecond
	msgs := runTestsAndReadAll(t, tests, &Config{CustomGracePeriod: &gracePeriod})

	// Tell the test to continue even though Run has already returned. The output stream should
	// still be writable so as to avoid a panic when the test writes to it (https://crbug.com/853406),
	// but they are dropped.
	cont <- true
	if completed := <-done; !completed {
		t.Error("Test function didn't complete")
	}

	// An error is written with a goroutine dump.
	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: "pkg.Test", Error: &protocol.Error{Reason: "Test did not return on timeout (see log for goroutine dump)"}},
		&protocol.EntityLogEvent{EntityName: "pkg.Test", Text: "Dumping all goroutines"},
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

	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		// Log message from the goroutine is not reported.
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

var bundleName = filepath.Base(os.Args[0])

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
		pre            *testPre
		preTestAction  action // Config.TestHook
		prepareAction  action // Precondition.Prepare
		testAction     action // Test.Func
		closeAction    action // Precondition.Close
		postTestAction action // Return of Config.TestHook
	}

	// pre1 and pre2 are preconditions used in tests. prepareFunc and closeFunc
	// are filled in each subtest.
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}

	evcmp := cmp.Comparer(func(a, b *protocol.EntityStartEvent) bool {
		return a.GetEntity().GetName() == b.GetEntity().GetName()
	})

	for _, tc := range []struct {
		name  string
		tests []testBehavior
		want  []proto.Message
	}{
		{
			name: "no precondition",
			tests: []testBehavior{
				{nil, pass, noCall, pass, noCall, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "passes",
			tests: []testBehavior{
				{pre1, pass, pass, pass, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "pretest fails",
			tests: []testBehavior{
				{pre1, doError, noCall, noCall, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "preTest: Intentional error"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "pretest panics",
			tests: []testBehavior{
				{pre1, doPanic, noCall, noCall, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "Panic: preTest: Intentional panic"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "prepare fails",
			tests: []testBehavior{
				{pre1, pass, doError, noCall, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "[Precondition failure] prepare: Intentional error"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "prepare panics",
			tests: []testBehavior{
				{pre1, pass, doPanic, noCall, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "[Precondition failure] Panic: prepare: Intentional panic"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
			},
		},
		{
			name: "same precondition",
			tests: []testBehavior{
				{pre1, pass, pass, pass, noCall, pass},
				{pre1, pass, pass, pass, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "1"}},
				&protocol.EntityLogEvent{EntityName: "1", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "1"},
			},
		},
		{
			name: "different preconditions",
			tests: []testBehavior{
				{pre1, pass, pass, pass, pass, pass},
				{pre2, pass, pass, pass, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "0", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "1"}},
				&protocol.EntityLogEvent{EntityName: "1", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Preparing precondition "pre2"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Closing precondition "pre2"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "1"},
			},
		},
		{
			name: "first prepare fails",
			tests: []testBehavior{
				{pre1, pass, doError, noCall, noCall, pass},
				{pre1, pass, pass, pass, pass, pass},
			},
			want: []proto.Message{
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "0"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "0", Text: `Preparing precondition "pre1"`},
				&protocol.EntityErrorEvent{EntityName: "0", Error: &protocol.Error{Reason: "[Precondition failure] prepare: Intentional error"}},
				&protocol.EntityLogEvent{EntityName: "0", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "0"},
				&protocol.EntityStartEvent{Entity: &protocol.Entity{Name: "1"}},
				&protocol.EntityLogEvent{EntityName: "1", Text: "preTest: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Preparing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "prepare: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "test: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: `Closing precondition "pre1"`},
				&protocol.EntityLogEvent{EntityName: "1", Text: "close: OK"},
				&protocol.EntityLogEvent{EntityName: "1", Text: "postTest: OK"},
				&protocol.EntityEndEvent{EntityName: "1"},
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
				TestHook: func(ctx context.Context, s *testing.TestHookState) func(context.Context, *testing.TestHookState) {
					doAction(s, currentBehavior(s).preTestAction, "preTest")
					return func(ctx context.Context, s *testing.TestHookState) {
						doAction(s, currentBehavior(s).postTestAction, "postTest")
					}
				},
			}
			msgs := runTestsAndReadAll(t, tests, pcfg)
			if diff := cmp.Diff(msgs, tc.want, evcmp); diff != "" {
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

	for _, tc := range []struct {
		name         string
		mode         DownloadMode
		numDownloads int
	}{
		{"batch", DownloadBatch, 1},
		{"lazy", DownloadLazy, 2},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			ds, err := devservertest.NewServer(devservertest.Files([]*devservertest.File{
				{URL: file1URL, Data: []byte(file1Data)},
				{URL: file2URL, Data: []byte(file2Data)},
				// file3.txt is missing.
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer ds.Close()

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

			numDownloads := 0

			pcfg := &Config{
				DataDir: dataDir,
				Features: dep.Features{
					Software: &dep.SoftwareFeatures{
						Available:   []string{"dep2"},
						Unavailable: []string{"dep1"},
					},
				},
				Devservers:   []string{ds.URL},
				DownloadMode: tc.mode,
				BeforeDownload: func(ctx context.Context) {
					numDownloads++
				},
			}

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
					Name: "example.Test2",
					Pkg:  "pkg",
					Func: func(ctx context.Context, s *testing.State) {
						fp := filepath.Join(dataDir, file3Path+testing.ExternalErrorSuffix)
						_, err := os.Stat(fp)
						switch tc.mode {
						case DownloadBatch:
							// In DownloadBatch mode, external data files for Test3 are already downloaded.
							if err != nil {
								t.Errorf("In Test2: %v; want present", err)
							}
						case DownloadLazy:
							// In DownloadLazy mode, external data files for Test3 are not downloaded yet.
							if err == nil {
								t.Errorf("In Test2: %s exists; want missing", fp)
							} else if !os.IsNotExist(err) {
								t.Errorf("In Test2: %v; want missing", err)
							}
						}
					},
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

			msgs := runTestsAndReadAll(t, tests, pcfg)

			want := []proto.Message{
				&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
				&protocol.EntityEndEvent{EntityName: tests[0].Name, Skip: &protocol.Skip{Reasons: []string{"missing SoftwareDeps: dep1"}}},
				&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
				&protocol.EntityEndEvent{EntityName: tests[1].Name},
				&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
				&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "Required data file file3.txt missing: failed to download gs://bucket/file3.txt: file does not exist"}},
				&protocol.EntityEndEvent{EntityName: tests[2].Name},
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

			if numDownloads != tc.numDownloads {
				t.Errorf("Unexpected number of download attempts: got %d, want %d", numDownloads, tc.numDownloads)
			}
		})
	}
}

func TestRunFixture(t *gotesting.T) {
	const (
		val1 = "val1"
		val2 = "val2"
	)
	fixt1 := &testing.Fixture{
		Name: "fixt1",
		Impl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				testcontext.Log(ctx, "fixt1 SetUp")
				return val1
			}),
			withReset(func(ctx context.Context) error {
				testcontext.Log(ctx, "fixt1 Reset")
				return nil
			}),
			withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt1 PreTest")
			}),
			withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt1 PostTest")
			}),
			withTearDown(func(ctx context.Context, s *testing.FixtState) {
				testcontext.Log(ctx, "fixt1 TearDown")
			})),
	}
	fixt2 := &testing.Fixture{
		Name: "fixt2",
		Impl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				testcontext.Log(ctx, "fixt2 SetUp")
				return val2
			}),
			withReset(func(ctx context.Context) error {
				testcontext.Log(ctx, "fixt2 Reset")
				return nil
			}),
			withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt2 PreTest")
			}),
			withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt2 PostTest")
			}),
			withTearDown(func(ctx context.Context, s *testing.FixtState) {
				testcontext.Log(ctx, "fixt2 TearDown")
			})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.Fixture{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name: "pkg.Test0",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 0")
			if val := s.FixtValue(); val != nil {
				t.Errorf("pkg.Test0: FixtValue() = %v; want nil", val)
			}
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 1")
			if val := s.FixtValue(); val != val1 {
				t.Errorf("pkg.Test1: FixtValue() = %v; want %v", val, val1)
			}
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 2")
			if val := s.FixtValue(); val != val2 {
				t.Errorf("pkg.Test2: FixtValue() = %v; want %v", val, val2)
			}
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []proto.Message{
		// pkg.Test0 simply runs.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Test 0"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		// pkg.Test1 depends on fixt1.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "fixt1 SetUp"},
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "fixt1 PreTest"},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "Test 1"},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "fixt1 PostTest"},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		// fixt1 is reset.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "fixt1 Reset"},
		// pkg.Test2 depends on fixt2, which in turn depends on fixt1.
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "fixt2 SetUp"},
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt1 PreTest"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt2 PreTest"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "Test 2"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt2 PostTest"},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "fixt1 PostTest"},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// fixt1 and fixt2 are torn down.
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "fixt2 TearDown"},
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "fixt1 TearDown"},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureSetUpFailure(t *gotesting.T) {
	fixt1 := &testing.Fixture{
		Name: "fixt1",
		Impl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Setup failure 1")
				s.Error("Setup failure 2")
				return nil
			}),
			withReset(func(ctx context.Context) error {
				testcontext.Log(ctx, "fixt1 Reset")
				return nil
			}),
			withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt1 PreTest")
			}),
			withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt1 PostTest")
			}),
			withTearDown(func(ctx context.Context, s *testing.FixtState) {
				testcontext.Log(ctx, "fixt1 TearDown")
			})),
	}
	fixt2 := &testing.Fixture{
		Name: "fixt2",
		Impl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				testcontext.Log(ctx, "fixt2 SetUp")
				return nil
			}),
			withReset(func(ctx context.Context) error {
				testcontext.Log(ctx, "fixt2 Reset")
				return nil
			}),
			withPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt2 PreTest")
			}),
			withPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				testcontext.Log(ctx, "fixt2 PostTest")
			}),
			withTearDown(func(ctx context.Context, s *testing.FixtState) {
				testcontext.Log(ctx, "fixt2 TearDown")
			})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.Fixture{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name: "pkg.Test0",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 0")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 1")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 2")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []proto.Message{
		// pkg.Test0 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Test 0"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		// fixt1 fails to set up.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: fixt1.Name, Error: &protocol.Error{Reason: "Setup failure 1"}},
		&protocol.EntityErrorEvent{EntityName: fixt1.Name, Error: &protocol.Error{Reason: "Setup failure 2"}},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
		// All tests depending on fixt1 fail.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[1].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 1"}},
		&protocol.EntityErrorEvent{EntityName: tests[1].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 2"}},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 1"}},
		&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "[Fixture failure] fixt1: Setup failure 2"}},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureRemoteSetUpFailure(t *gotesting.T) {
	cfg := &Config{
		StartFixtureName: "remoteFixt",
		StartFixtureImpl: newFakeFixture(
			withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Error("Remote failure")
				return nil
			})),
	}
	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "remoteFixt",
		Func: func(ctx context.Context, s *testing.State) {
			t.Errorf("pkg.Test run unexpectedly")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityErrorEvent{Error: &protocol.Error{Reason: "[Fixture failure] remoteFixt: Remote failure"}, EntityName: "pkg.Test"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureResetFailure(t *gotesting.T) {
	fixt1 := &testing.Fixture{
		Name: "fixt1",
		Impl: newFakeFixture(withReset(func(ctx context.Context) error {
			testcontext.Log(ctx, "Reset 1")
			return errors.New("failure")
		})),
	}
	fixt2 := &testing.Fixture{
		Name: "fixt2",
		Impl: newFakeFixture(withReset(func(ctx context.Context) error {
			testcontext.Log(ctx, "Reset 2")
			return nil
		})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.Fixture{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name: "pkg.Test0",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 0")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 1")
		},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test 2")
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []proto.Message{
		// pkg.Test0 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Test 0"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		// fixt1 sets up successfully.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		// pkg.Test1 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "Test 1"},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		// fixt1 fails to reset. It is torn down.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Fixture failed to reset: failure; recovering"},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		// fixt2 sets up successfully.
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		// pkg.Test2 runs successfully.
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[2].Name, Text: "Test 2"},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// Fixtures are torn down.
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureMinimumReset(t *gotesting.T) {
	fixt1 := &testing.Fixture{
		Name: "fixt1",
		Impl: newFakeFixture(withReset(func(ctx context.Context) error {
			testcontext.Log(ctx, "Reset 1")
			return nil
		})),
	}
	fixt2 := &testing.Fixture{
		Name: "fixt2",
		Impl: newFakeFixture(withReset(func(ctx context.Context) error {
			testcontext.Log(ctx, "Reset 2")
			return nil
		})),
		Parent: "fixt1",
	}
	fixt3 := &testing.Fixture{
		Name: "fixt3",
		Impl: newFakeFixture(withReset(func(ctx context.Context) error {
			testcontext.Log(ctx, "Reset 3")
			return nil
		})),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.Fixture{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
			fixt3.Name: fixt3,
		},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test0",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test3",
		Fixture: "fixt2",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test4",
		Fixture: "fixt2",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test5",
		Fixture: "fixt3",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test6",
		Fixture: "fixt3",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []proto.Message{
		// pkg.Test0 runs.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		// fixt1 starts.
		&protocol.EntityStartEvent{Entity: fixt1.EntityProto()},
		// pkg.Test1 runs.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		// fixt1 is reset for pkg.Test2.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		// pkg.Test2 runs.
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// fixt1 is reset for pkg.Test3. fixt2 is NOT reset.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		// fixt2 starts.
		&protocol.EntityStartEvent{Entity: fixt2.EntityProto()},
		// pkg.Test3 runs.
		&protocol.EntityStartEvent{Entity: tests[3].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[3].Name},
		// fixt1 and fixt2 are reset for pkg.Test4.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt2.Name, Text: "Reset 2"},
		// pkg.Test4 runs.
		&protocol.EntityStartEvent{Entity: tests[4].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[4].Name},
		// fixt2 ends.
		&protocol.EntityEndEvent{EntityName: fixt2.Name},
		// fixt1 is reset for pkg.Test5. fixt2 is NOT reset.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		// fixt3 starts.
		&protocol.EntityStartEvent{Entity: fixt3.EntityProto()},
		// pkg.Test5 runs.
		&protocol.EntityStartEvent{Entity: tests[5].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[5].Name},
		// fixt1 and fixt3 are reset for pkg.Test5.
		&protocol.EntityLogEvent{EntityName: fixt1.Name, Text: "Reset 1"},
		&protocol.EntityLogEvent{EntityName: fixt3.Name, Text: "Reset 3"},
		// pkg.Test6 runs.
		&protocol.EntityStartEvent{Entity: tests[6].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[6].Name},
		// fixt3 and fixt1 end. They are NOT reset.
		&protocol.EntityEndEvent{EntityName: fixt3.Name},
		&protocol.EntityEndEvent{EntityName: fixt1.Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureMissing(t *gotesting.T) {
	fixt1 := &testing.Fixture{
		Name:   "fixt1",
		Impl:   newFakeFixture(),
		Parent: "fixt0", // no such fixture
	}
	fixt2 := &testing.Fixture{
		Name:   "fixt2",
		Impl:   newFakeFixture(),
		Parent: "fixt1",
	}
	cfg := &Config{
		Fixtures: map[string]*testing.Fixture{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test0",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test1",
		Fixture: "fixt1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}, {
		Name:    "pkg.Test2",
		Fixture: "fixt2",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, cfg)

	want := []proto.Message{
		// Orphan tests are reported first, sorted by name.
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[1].Name, Error: &protocol.Error{Reason: "Fixture \"fixt0\" not found"}},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
		&protocol.EntityStartEvent{Entity: tests[2].EntityProto()},
		&protocol.EntityErrorEvent{EntityName: tests[2].Name, Error: &protocol.Error{Reason: "Fixture \"fixt0\" not found"}},
		&protocol.EntityEndEvent{EntityName: tests[2].Name},
		// Valid tests are run.
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunFixtureVars(t *gotesting.T) {
	const (
		declaredVarName   = "declared"
		declaredVarValue  = "foo"
		undeclaredVarName = "undeclared"
	)

	fixt := &testing.Fixture{
		Name: "fixt",
		Impl: newFakeFixture(withSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			func() {
				defer func() {
					if recover() != nil {
						t.Errorf("Failed to access variable %q", declaredVarName)
					}
				}()
				if value, ok := s.Var(declaredVarName); !ok {
					t.Errorf("Variable %q not found", declaredVarName)
				} else if value != declaredVarValue {
					t.Errorf("Variable %q = %q; want %q", declaredVarName, value, declaredVarValue)
				}
			}()
			func() {
				// Brace for panic.
				defer func() {
					recover()
				}()
				s.Var(undeclaredVarName)
				t.Errorf("Variable %q could be accessed unexpectedly", undeclaredVarName)
			}()
			return nil
		})),
		Vars: []string{declaredVarName},
	}

	tests := []*testing.TestInstance{{
		Name:    "pkg.Test",
		Fixture: "fixt",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}}

	cfg := &Config{
		Fixtures: map[string]*testing.Fixture{fixt.Name: fixt},
		Features: dep.Features{
			Var: map[string]string{
				declaredVarName:   declaredVarValue,
				undeclaredVarName: "forbidden",
			},
		},
	}

	runTestsAndReadAll(t, tests, cfg)
}

func TestRunPrecondition(t *gotesting.T) {
	type data struct{}
	preData := &data{}

	// The test should be able to access the data via State.PreValue.
	tests := []*testing.TestInstance{{
		Name: "pkg.Test",
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

	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: `Preparing precondition "pre"`},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: `Closing precondition "pre"`},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
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

		testcontext.Log(pctx, "Log via PreCtx")

		if _, ok := testcontext.SoftwareDeps(pctx); !ok {
			t.Error("ContextSoftwareDeps unavailable")
		}
		if _, ok := testcontext.OutDir(pctx); ok {
			t.Error("ContextOutDir available")
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

	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: `Preparing precondition "pre"`},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "Log via PreCtx"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
		&protocol.EntityStartEvent{Entity: tests[1].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: `Preparing precondition "pre"`},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: "Log via PreCtx"},
		&protocol.EntityLogEvent{EntityName: tests[1].Name, Text: `Closing precondition "pre"`},
		&protocol.EntityEndEvent{EntityName: tests[1].Name},
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
				TestHook: func(ctx context.Context, s *testing.TestHookState) func(context.Context, *testing.TestHookState) {
					onPhase(s, phasePreTestFunc)
					return func(ctx context.Context, s *testing.TestHookState) {
						onPhase(s, phasePostTestFunc)
					}
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
		Name: "pkg.Test",
		Func: func(ctx context.Context, s *testing.State) {
			testcontext.Log(ctx, "msg ", 1)
			testcontext.Logf(ctx, "msg %d", 2)
		},
		Timeout: time.Minute,
	}}

	msgs := runTestsAndReadAll(t, tests, &Config{})

	want := []proto.Message{
		&protocol.EntityStartEvent{Entity: tests[0].EntityProto()},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "msg 1"},
		&protocol.EntityLogEvent{EntityName: tests[0].Name, Text: "msg 2"},
		&protocol.EntityEndEvent{EntityName: tests[0].Name},
	}
	if diff := cmp.Diff(msgs, want); diff != "" {
		t.Error("Output mismatch (-got +want):\n", diff)
	}
}

func TestRunPlan(t *gotesting.T) {
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}
	fixt1 := &testing.Fixture{Name: "fixt1", Impl: newFakeFixture()}
	fixt2 := &testing.Fixture{Name: "fixt2", Impl: newFakeFixture(), Parent: "fixt1"}
	cfg := &Config{
		Features: dep.Features{
			Software: &dep.SoftwareFeatures{
				Available:   []string{"yes"},
				Unavailable: []string{"no"},
			},
		},
		Fixtures: map[string]*testing.Fixture{
			fixt1.Name: fixt1,
			fixt2.Name: fixt2,
		},
	}

	for _, tc := range []struct {
		name      string
		tests     []*testing.TestInstance
		wantOrder []string
	}{
		{
			name: "pre",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test6", Pre: pre2},
				{Name: "pkg.Test5", Pre: pre1},
				{Name: "pkg.Test4"},
				{Name: "pkg.Test3"},
				{Name: "pkg.Test2", Pre: pre1},
				{Name: "pkg.Test1", Pre: pre2},
			},
			wantOrder: []string{
				// Sorted by (precondition name, test name).
				"pkg.Test3",
				"pkg.Test4",
				"pkg.Test2",
				"pkg.Test5",
				"pkg.Test1",
				"pkg.Test6",
			},
		},
		{
			name: "fixt",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test6", Fixture: "fixt2"},
				{Name: "pkg.Test5", Fixture: "fixt1"},
				{Name: "pkg.Test4"},
				{Name: "pkg.Test3"},
				{Name: "pkg.Test2", Fixture: "fixt1"},
				{Name: "pkg.Test1", Fixture: "fixt2"},
			},
			wantOrder: []string{
				"pkg.Test3",
				"pkg.Test4",
				"fixt1",
				"pkg.Test2",
				"pkg.Test5",
				"fixt2",
				"pkg.Test1",
				"pkg.Test6",
			},
		},
		{
			name: "fixt_and_pre",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test1", Pre: pre1},
				{Name: "pkg.Test2", Fixture: "fixt1"},
				{Name: "pkg.Test3"},
			},
			wantOrder: []string{
				"pkg.Test3",
				"fixt1",
				"pkg.Test2",
				"pkg.Test1",
			},
		},
		{
			name: "deps",
			tests: []*testing.TestInstance{
				{Name: "pkg.Test4", SoftwareDeps: []string{"yes"}},
				{Name: "pkg.Test3", SoftwareDeps: []string{"no"}},
				{Name: "pkg.Test2", SoftwareDeps: []string{"no"}, Pre: pre1},
				{Name: "pkg.Test1", SoftwareDeps: []string{"no"}, Pre: pre2},
			},
			wantOrder: []string{
				// Skipped tests come first.
				"pkg.Test1",
				"pkg.Test2",
				"pkg.Test3",
				"pkg.Test4",
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			msgs := runTestsAndReadAll(t, tc.tests, cfg)
			var order []string
			for _, msg := range msgs {
				if msg, ok := msg.(*protocol.EntityStartEvent); ok {
					order = append(order, msg.GetEntity().GetName())
				}
			}
			if diff := cmp.Diff(order, tc.wantOrder); diff != "" {
				t.Error("Test order mismatch (-got +want):\n", diff)
			}
		})
	}
}
