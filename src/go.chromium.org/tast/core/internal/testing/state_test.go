// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing_test

import (
	"context"
	"fmt"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/chromiumos/config/go/api"
	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/testutil"
)

// outputSink is an implementation of OutputStream for unit tests.
type outputSink struct {
	mu   sync.Mutex
	Data outputData
}

type outputData struct {
	Logs     []string
	FullLogs []string
	Errs     []*protocol.Error
}

func (r *outputSink) Log(level logging.Level, ts time.Time, msg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if level >= logging.LevelInfo {
		r.Data.Logs = append(r.Data.Logs, msg)
	}
	r.Data.FullLogs = append(r.Data.FullLogs, msg)
	return nil
}

func (r *outputSink) Error(e *protocol.Error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Data.Errs = append(r.Data.Errs, e)
	return nil
}

var outputDataCmpOpts = []cmp.Option{
	protocmp.Transform(),
	protocmp.IgnoreFields(&protocol.Error{}, "location"),
}

func TestLog(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	s.Log("msg ", 1)
	s.Logf("msg %d", 2)
	s.VLog("msg ", 3)
	s.VLogf("msg %d", 4)
	exp := outputData{Logs: []string{"msg 1", "msg 2"}, FullLogs: []string{"msg 1", "msg 2", "msg 3", "msg 4"}}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestLogInvalidUTF8(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	const invalidUTF8 = "\xed"
	s.Log("msg ", 1, invalidUTF8)
	s.Logf("msg %d %v", 2, invalidUTF8)
	s.VLog("msg ", 3, invalidUTF8)
	s.VLogf("msg %d %v", 4, invalidUTF8)
	for _, l := range out.Data.Logs {
		if strings.Contains(l, invalidUTF8) {
			t.Errorf("out.Data.Logs message %q has invalid UTF-8 characters", l)
		}
	}
	for _, l := range out.Data.FullLogs {
		if strings.Contains(l, invalidUTF8) {
			t.Errorf("out.Data.FullLogs message %q has invalid UTF-8 characters", l)
		}
	}
}

func TestNestedRun(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	ctx := context.Background()

	s.Run(ctx, "p1", func(ctx context.Context, s *testing.State) {
		s.Log("msg ", 1)
		s.VLog("vmsg ", 1)

		s.Run(ctx, "p2", func(ctx context.Context, s *testing.State) {
			s.Log("msg ", 2)
			s.VLog("vmsg ", 2)
		})

		s.Log("msg ", 3)
		s.VLog("vmsg ", 3)
	})

	s.Log("msg ", 4)
	s.VLog("vmsg ", 4)

	exp := outputData{Logs: []string{
		"Starting subtest p1",
		"msg 1",
		"Starting subtest p1/p2",
		"msg 2",
		"msg 3",
		"msg 4",
	}, FullLogs: []string{
		"Starting subtest p1",
		"msg 1",
		"vmsg 1",
		"Starting subtest p1/p2",
		"msg 2",
		"vmsg 2",
		"msg 3",
		"vmsg 3",
		"msg 4",
		"vmsg 4",
	}}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestRunReturn(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	ctx := context.Background()

	if res := s.Run(ctx, "p1", func(ctx context.Context, s *testing.State) {
		s.Fatal("fail")
	}); res != false {
		t.Error("Expected failure to return false")
	}

	if res := s.Run(ctx, "p2", func(ctx context.Context, s *testing.State) {
		s.Log("ok")
		s.VLog("vok")
	}); res != true {
		t.Error("Expected success to return true")
	}

	exp := outputData{
		Logs: []string{
			"Starting subtest p1",
			"Starting subtest p2",
			"ok",
		},
		FullLogs: []string{
			"Starting subtest p1",
			"Starting subtest p2",
			"ok",
			"vok",
		},
		Errs: []*protocol.Error{
			{Reason: "p1: fail"},
		},
	}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestParallelRun(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	s.Run(ctx, "r", func(ctx context.Context, s *testing.State) {
		go func() {
			s.Run(ctx, "t1", func(ctx context.Context, s *testing.State) {
				s.Log("msg ", 1)
			})
			wg.Done()
		}()
		go func() {
			s.Run(ctx, "t2", func(ctx context.Context, s *testing.State) {
				s.Log("msg ", 2)
			})
			wg.Done()
		}()
	})
	wg.Wait()

	if len(out.Data.Errs) != 0 || len(out.Data.Logs) != 5 || out.Data.Logs[0] != "Starting subtest r" {
		t.Fatalf("Bad test report: %+v", out.Data)
	}

	// Check that both messages appear and are sequential. Ordering between
	// subtests is random.

	hasOutput := func(id string) bool {
		var relatedLogs []string
		for _, log := range out.Data.Logs[1:] {
			if strings.HasSuffix(log, id) {
				relatedLogs = append(relatedLogs, log)
			}
		}
		return len(relatedLogs) == 2 &&
			strings.HasPrefix(relatedLogs[0], "Starting subtest") &&
			strings.HasPrefix(relatedLogs[1], "msg")
	}

	if !hasOutput("1") || !hasOutput("2") {
		t.Errorf("Bad test report: %+v", out.Data)
	}
}

func TestReportError(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	// Keep these lines next to each other (see below comparison).
	s.Error("error ", 1)
	s.Errorf("error %d", 2)

	if len(out.Data.Logs) != 0 || len(out.Data.Errs) != 2 {
		t.Fatalf("Bad test report: %+v", out.Data)
	}

	e0, e1 := out.Data.Errs[0], out.Data.Errs[1]
	if e0 == nil || e1 == nil {
		t.Fatal("Got nil error(s)")
	}
	if act, exp := []string{e0.Reason, e1.Reason}, []string{"error 1", "error 2"}; !reflect.DeepEqual(act, exp) {
		t.Errorf("Got reasons %v; want %v", act, exp)
	}
	if _, fn, _, _ := runtime.Caller(0); e0.GetLocation().GetFile() != fn || e1.GetLocation().GetFile() != fn {
		t.Errorf("Got filenames %q and %q; want %q", e0.GetLocation().GetFile(), e1.GetLocation().GetFile(), fn)
	}
	if e0.GetLocation().GetLine()+1 != e1.GetLocation().GetLine() {
		t.Errorf("Got non-sequential line numbers %d and %d", e0.GetLocation().GetLine(), e1.GetLocation().GetLine())
	}

	for _, e := range []*protocol.Error{e0, e1} {
		lines := strings.Split(e.GetLocation().GetStack(), "\n")
		if len(lines) < 2 {
			t.Errorf("Stack trace %q contains fewer than 2 lines", string(e.GetLocation().GetStack()))
			continue
		}
		if exp := "error "; !strings.HasPrefix(lines[0], exp) {
			t.Errorf("First line of stack trace %q doesn't start with %q", string(e.GetLocation().GetStack()), exp)
		}
		if exp := fmt.Sprintf("\tat go.chromium.org/tast/core/internal/testing_test.TestReportError (%s:%d)", filepath.Base(e.GetLocation().GetFile()), e.GetLocation().GetLine()); lines[1] != exp {
			t.Errorf("Second line of stack trace %q doesn't match %q", string(e.GetLocation().GetStack()), exp)
		}
	}
}

func TestInheritError(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())

	s1 := root.NewTestState()
	if s1.HasError() {
		t.Error("First State: HasError()=true initially; want false")
	}
	s1.Error("Failure")
	if !s1.HasError() {
		t.Error("First State: HasError()=false after s1.Error; want true")
	}

	// The second state should be aware of the error reported to the first state.
	s2 := root.NewTestState()
	if !s2.HasError() {
		t.Error("Second State: HasError()=false initially; want true")
	}

	// Subtest State should not inherit the error status from the parent state.
	s2.Run(context.Background(), "subtest", func(ctx context.Context, s2s *testing.State) {
		if s2s.HasError() {
			t.Error("Subtest State: HasError()=true initially; want false")
		}
		s2s.Error("Failure")
		if !s2s.HasError() {
			t.Error("Subtest State: HasError()=false after s2s.Error; want true")
		}
	})
}

func TestReportErrorInPrecondition(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewPreState()

	// Keep these lines next to each other (see below comparison).
	s.Error("error ", 1)
	s.Errorf("error %d", 2)

	if len(out.Data.Logs) != 0 || len(out.Data.Errs) != 2 {
		t.Fatalf("Bad test report: %+v", out.Data)
	}

	e0, e1 := out.Data.Errs[0], out.Data.Errs[1]
	if e0 == nil || e1 == nil {
		t.Fatal("Got nil error(s)")
	}
	if act, exp := []string{e0.Reason, e1.Reason}, []string{"[Precondition failure] error 1", "[Precondition failure] error 2"}; !reflect.DeepEqual(act, exp) {
		t.Errorf("Got reasons %v; want %v", act, exp)
	}
	if _, fn, _, _ := runtime.Caller(0); e0.GetLocation().GetFile() != fn || e1.GetLocation().GetFile() != fn {
		t.Errorf("Got filenames %q and %q; want %q", e0.GetLocation().GetFile(), e1.GetLocation().GetFile(), fn)
	}
	if e0.GetLocation().GetLine()+1 != e1.GetLocation().GetLine() {
		t.Errorf("Got non-sequential line numbers %d and %d", e0.GetLocation().GetLine(), e1.GetLocation().GetLine())
	}

	for _, e := range []*protocol.Error{e0, e1} {
		lines := strings.Split(e.GetLocation().GetStack(), "\n")
		if len(lines) < 2 {
			t.Errorf("Stack trace %q contains fewer than 2 lines", string(e.GetLocation().GetStack()))
			continue
		}
		if exp := "[Precondition failure] error "; !strings.HasPrefix(lines[0], exp) {
			t.Errorf("First line of stack trace %q doesn't start with %q", string(e.GetLocation().GetStack()), exp)
		}
		if exp := fmt.Sprintf("\tat go.chromium.org/tast/core/internal/testing_test.TestReportErrorInPrecondition (%s:%d)", filepath.Base(e.GetLocation().GetFile()), e.GetLocation().GetLine()); lines[1] != exp {
			t.Errorf("Second line of stack trace %q doesn't match %q", string(e.GetLocation().GetStack()), exp)
		}
	}
}

func errorFunc() error {
	return errors.New("meow")
}

func TestExtractErrorSimple(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	err := errorFunc()
	s.Error(err)

	if len(out.Data.Logs) != 0 || len(out.Data.Errs) != 1 {
		t.Fatalf("Bad test report: %+v", out.Data)
	}

	e := out.Data.Errs[0]

	if exp := "meow"; e.Reason != exp {
		t.Errorf("Error message %q is not %q", e.Reason, exp)
	}
	if exp := "meow\n\tat go.chromium.org/tast/core/internal/testing_test.TestExtractErrorSimple"; !strings.HasPrefix(e.GetLocation().GetStack(), exp) {
		t.Errorf("Stack trace %q doesn't start with %q", e.GetLocation().GetStack(), exp)
	}
	if exp := "meow\n\tat go.chromium.org/tast/core/internal/testing_test.errorFunc"; !strings.Contains(e.GetLocation().GetStack(), exp) {
		t.Errorf("Stack trace %q doesn't contain %q", e.GetLocation().GetStack(), exp)
	}
}

func TestExtractErrorHeuristic(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	err := errorFunc()
	s.Error("Failed something  :  ", err)
	s.Error("Failed something  ", err)
	s.Errorf("Failed something  :  %v", err)
	s.Errorf("Failed something  %v", err)

	if len(out.Data.Logs) != 0 || len(out.Data.Errs) != 4 {
		t.Fatalf("Bad test report: %+v", out.Data)
	}

	for _, e := range out.Data.Errs {
		if exp := "Failed something  "; !strings.HasPrefix(e.Reason, exp) {
			t.Errorf("Error message %q doesn't start with %q", e.Reason, exp)
		}
		if exp := "Failed something\n\tat go.chromium.org/tast/core/internal/testing_test.TestExtractErrorHeuristic"; !strings.HasPrefix(e.GetLocation().GetStack(), exp) {
			t.Errorf("Stack trace %q doesn't start with %q", e.GetLocation().GetStack(), exp)
		}
		if exp := "\nmeow\n\tat go.chromium.org/tast/core/internal/testing_test.errorFunc"; !strings.Contains(e.GetLocation().GetStack(), exp) {
			t.Errorf("Stack trace %q doesn't contain %q", e.GetLocation().GetStack(), exp)
		}
	}
}

func TestRunUsePrefix(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	ctx := context.Background()
	s.Run(ctx, "f1", func(ctx context.Context, s *testing.State) {
		s.Run(ctx, "f2", func(ctx context.Context, s *testing.State) {
			s.Errorf("error %s", "msg")
		})
	})

	if !s.HasError() {
		t.Error("Test is not reporting error")
	}

	if len(out.Data.Logs) != 2 || len(out.Data.Errs) != 1 {
		t.Fatalf("Bad test report: %+v", out.Data)
	}

	exp := outputData{
		Logs: []string{
			"Starting subtest f1",
			"Starting subtest f1/f2",
		},
		FullLogs: []string{
			"Starting subtest f1",
			"Starting subtest f1/f2",
		},
		Errs: []*protocol.Error{
			{Reason: "f1/f2: error msg"},
		},
	}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestRunNonFatal(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer close(done)

		ctx := context.Background()
		s.Run(ctx, "f", func(ctx context.Context, s *testing.State) {
			s.Fatal("fatal msg")
		})

		died = false
	}()
	<-done

	if died {
		t.Error("Test stopped due to fail")
	}

	exp := outputData{
		Logs: []string{
			"Starting subtest f",
		},
		FullLogs: []string{
			"Starting subtest f",
		},
		Errs: []*protocol.Error{
			{Reason: "f: fatal msg"},
		},
	}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestFatal(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer close(done)
		s.Fatalf("fatal %s", "msg")
		died = false
	}()
	<-done

	if !died {
		t.Fatal("Test continued after call to Fatalf")
	}

	exp := outputData{
		Errs: []*protocol.Error{
			{Reason: "fatal msg"},
		},
	}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestFatalInPrecondition(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewPreState()

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer close(done)
		s.Fatalf("fatal %s", "msg")
		died = false
	}()
	<-done

	if !died {
		t.Fatal("Test continued after call to Fatalf")
	}

	exp := outputData{
		Errs: []*protocol.Error{
			{Reason: "[Precondition failure] fatal msg"},
		},
	}
	if diff := cmp.Diff(out.Data, exp, outputDataCmpOpts...); diff != "" {
		t.Errorf("Bad test report (-got +want):\n%s", diff)
	}
}

func TestDataPathDeclared(t *gotesting.T) {
	const (
		dataDir = "/tmp/data"
	)
	test := testing.TestInstance{
		Timeout: time.Minute,
		Data:    []string{"foo", "foo/bar", "foo/baz"},
	}

	for _, tc := range []struct{ in, exp string }{
		{"foo", filepath.Join(dataDir, "foo")},
		{"foo/bar", filepath.Join(dataDir, "foo/bar")},
	} {
		var out outputSink
		root := testing.NewTestEntityRoot(&test, &testing.RuntimeConfig{DataDir: dataDir}, &out, testing.NewEntityCondition())
		s := root.NewTestState()
		if act := s.DataPath(tc.in); act != tc.exp {
			t.Errorf("DataPath(%q) = %q; want %q", tc.in, act, tc.exp)
		}
	}
}

func TestDataPathNotDeclared(t *gotesting.T) {
	var out outputSink
	test := testing.TestInstance{
		Timeout: time.Minute,
		Data:    []string{"foo"},
	}
	root := testing.NewTestEntityRoot(&test, &testing.RuntimeConfig{DataDir: "/data"}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	// Request an undeclared data path to cause a panic.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("DataPath did not panic")
			}
		}()
		s.DataPath("bar")
	}()
}

func TestDataPaths(t *gotesting.T) {
	const (
		dataDir = "/tmp/data"
	)
	test := testing.TestInstance{
		Timeout: time.Minute,
		Data:    []string{"foo", "foo/bar", "foo/baz"},
	}

	expected := make(map[string]string)
	for _, f := range test.Data {
		expected[f] = filepath.Join(dataDir, f)
	}

	var out outputSink
	root := testing.NewTestEntityRoot(&test, &testing.RuntimeConfig{DataDir: dataDir}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	got := s.DataPaths()
	if diff := cmp.Diff(got, expected); diff != "" {
		t.Fatalf("failed to get expected data paths (-got +want):\n%s", diff)
	}
}

func TestDataFileServer(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	const (
		file1   = "dir/file1.txt"
		file2   = "dir2/file2.txt"
		missing = "missing.txt"
		data1   = "first file"
	)
	if err := testutil.WriteFiles(td, map[string]string{
		file1: data1,
		file2: "second file",
	}); err != nil {
		t.Fatal(err)
	}

	test := testing.TestInstance{Data: []string{file1}}
	var out outputSink
	root := testing.NewTestEntityRoot(&test, &testing.RuntimeConfig{DataDir: td}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()

	get := func(p string) (string, error) {
		resp, err := http.Get(srv.URL + "/" + p)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", errors.New(resp.Status)
		}
		body, err := io.ReadAll(resp.Body)
		return string(body), err
	}

	if str, err := get(file1); err != nil {
		t.Errorf("GET %v failed: %v", file1, err)
	} else if str != data1 {
		t.Errorf("GET %v returned %q; want %q", file1, str, data1)
	}

	if str, err := get(missing); err == nil {
		t.Errorf("GET %v returned %q; want error", missing, str)
	}
	if s.HasError() {
		t.Error("State contains error after requesting missing file")
	}

	if str, err := get(file2); err == nil {
		t.Errorf("GET %v returned %q; want error", file2, str)
	}
	if s.HasError() {
		t.Error("State contains error after requesting unregistered file")
	}
}

func TestVars(t *gotesting.T) {
	const (
		validName = "valid" // registered by test and provided
		unsetName = "unset" // registered by test but not provided at runtime
		unregName = "unreg" // not registered by test but provided at runtime

		validValue = "valid value"
		unregValue = "unreg value"
	)

	test := &testing.TestInstance{Vars: []string{validName, unsetName}}
	cfg := &testing.RuntimeConfig{Vars: map[string]string{validName: validValue, unregName: unregValue}}
	var out outputSink
	root := testing.NewTestEntityRoot(test, cfg, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	for _, tc := range []struct {
		req   bool   // if true, call RequiredVar instead of Var
		name  string // name to pass to Var/RequiredVar
		value string // expected variable value to be returned
		ok    bool   // expected 'ok' return value (only used if req is false)
		fatal bool   // if true, test should be aborted
	}{
		{false, validName, validValue, true, false},
		{false, unsetName, "", false, false},
		{false, unregName, "", false, true},
		{true, validName, validValue, false, false},
		{true, unsetName, "", false, true},
		{true, unregName, "", false, true},
	} {
		funcCall := fmt.Sprintf("Var(%q)", tc.name)
		if tc.req {
			funcCall = fmt.Sprintf("RequiredVar(%q)", tc.name)
		}

		// Call the function in a goroutine since it may call runtime.Goexit() via Fatal.
		finished := false
		done := make(chan struct{})
		go func() {
			defer func() {
				recover()
				close(done)
			}()
			if tc.req {
				if value := s.RequiredVar(tc.name); value != tc.value {
					t.Errorf("%s = %q; want %q", funcCall, value, tc.value)
				}
			} else {
				if value, ok := s.Var(tc.name); value != tc.value || ok != tc.ok {
					t.Errorf("%s = (%q, %v); want (%q, %v)", funcCall, value, ok, tc.value, tc.ok)
				}
			}
			finished = true
		}()
		<-done

		if !finished && !tc.fatal {
			t.Error(funcCall, " aborted unexpectedly")
		} else if finished && tc.fatal {
			t.Error(funcCall, " succeeded unexpectedly")
		}
	}
}

func TestMeta(t *gotesting.T) {
	meta := testing.Meta{
		TastPath:       "/foo/bar",
		Target:         "example.net",
		RunFlags:       []string{"-foo", "-bar"},
		ListFlags:      []string{"-lflag"},
		ConnectionSpec: "example.net",
	}
	getMeta := func(test *testing.TestInstance, cfg *testing.RuntimeConfig) (meta *testing.Meta, ok bool) {
		var out outputSink
		root := testing.NewTestEntityRoot(test, cfg, &out, testing.NewEntityCondition())
		s := root.NewTestState()

		// Meta can panic, so run with recover.
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		return s.Meta(), true
	}

	const (
		metaTest    = "meta.Test"
		nonMetaTest = "pkg.Test"
	)

	// Meta info should be provided to tests in the "meta" package.
	if m, ok := getMeta(&testing.TestInstance{Name: metaTest}, &testing.RuntimeConfig{RemoteData: &testing.RemoteData{Meta: &meta}}); !ok {
		t.Errorf("Meta() panicked for %v", metaTest)
	} else if m == nil {
		t.Errorf("Meta() = nil for %v", metaTest)
	} else if !reflect.DeepEqual(*m, meta) {
		t.Errorf("Meta() = %+v for %v; want %+v", *m, metaTest, meta)
	}

	// Tests not in the "meta" package shouldn't have access to meta info.
	if m, ok := getMeta(&testing.TestInstance{Name: nonMetaTest}, &testing.RuntimeConfig{RemoteData: &testing.RemoteData{Meta: &meta}}); ok {
		t.Errorf("Meta() didn't panic for %v", nonMetaTest)
	} else if m != nil {
		t.Errorf("Meta() = %+v for %v", *m, nonMetaTest)
	}

	// Check that newState doesn't crash or somehow get a non-nil Meta struct when initially passed a nil struct.
	if m, ok := getMeta(&testing.TestInstance{Name: metaTest}, &testing.RuntimeConfig{}); ok {
		t.Error("Meta() didn't panic for nil info")
	} else if m != nil {
		t.Errorf("Meta() = %+v despite nil info", *m)
	}
}

func TestRPCHint(t *gotesting.T) {
	hint := testing.NewRPCHint("/path/to/bundles", map[string]string{"var": "value"})
	getHint := func(test *testing.TestInstance, cfg *testing.RuntimeConfig) (hint *testing.RPCHint, ok bool) {
		var out outputSink
		root := testing.NewTestEntityRoot(test, cfg, &out, testing.NewEntityCondition())
		s := root.NewTestState()

		// RPCHint can panic, so run with recover.
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		return s.RPCHint(), true
	}

	const (
		remoteTest = "do.Remotely"
		localTest  = "do.Locally"
	)

	// RPCHint should be provided to remote tests.
	if h, ok := getHint(&testing.TestInstance{Name: remoteTest}, &testing.RuntimeConfig{RemoteData: &testing.RemoteData{RPCHint: hint}}); !ok {
		t.Errorf("RPCHint() panicked for %v", remoteTest)
	} else if h == nil {
		t.Errorf("RPCHint() = nil for %v", remoteTest)
	} else if !reflect.DeepEqual(h, hint) {
		t.Errorf("RPCHint() = %+v for %v; want %+v", *h, remoteTest, *hint)
	}

	// Local tests shouldn't have access to RPCHint.
	if h, ok := getHint(&testing.TestInstance{Name: localTest}, &testing.RuntimeConfig{}); ok {
		t.Errorf("RPCHint() didn't panic for %v", localTest)
	} else if h != nil {
		t.Errorf("RPCHint() = %+v for %v", *h, localTest)
	}
}

func TestDUT(t *gotesting.T) {
	callDUT := func(test *testing.TestInstance, cfg *testing.RuntimeConfig) (ok bool) {
		var out outputSink
		root := testing.NewTestEntityRoot(test, cfg, &out, testing.NewEntityCondition())
		s := root.NewTestState()

		// DUT can panic, so run with recover.
		// so run this in a goroutine to isolate it from the test.
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		s.DUT()
		return true
	}

	const (
		remoteTest = "do.Remotely"
		localTest  = "do.Locally"
	)

	// DUT should be provided to remote tests.
	if ok := callDUT(&testing.TestInstance{Name: remoteTest}, &testing.RuntimeConfig{RemoteData: &testing.RemoteData{}}); !ok {
		t.Errorf("DUT() panicked for %v", remoteTest)
	}

	// Local tests shouldn't have access to DUT.
	if ok := callDUT(&testing.TestInstance{Name: localTest}, &testing.RuntimeConfig{}); ok {
		t.Errorf("DUT() didn't panic for %v", localTest)
	}
}

func TestCloudStorage(t *gotesting.T) {
	want := testing.NewCloudStorage(nil, "", "", "", "", "", "")

	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Name: "example.Test"}, &testing.RuntimeConfig{CloudStorage: want}, &out, testing.NewEntityCondition())
	s := root.NewTestState()
	got := s.CloudStorage()

	if got != want {
		t.Errorf("CloudStorage returned %v; want %v", got, want)
	}
}

func TestDUtLabConfig(t *gotesting.T) {
	var out outputSink
	chromeOSDUTID := "chromeos_dut"
	androidDUTID := "android_dut"
	devboardDUTID := "devboard_dut"
	root := testing.NewTestEntityRoot(
		&testing.TestInstance{Timeout: time.Minute},
		&testing.RuntimeConfig{
			DUTLabConfig: &frameworkprotocol.DUTLabConfig{
				ChromeOSDUTLabConfig: map[string]*labapi.Dut{
					"": {
						Id: &labapi.Dut_Id{Value: chromeOSDUTID},
					},
				},
				AndroidDUTLabConfig: map[string]*labapi.Dut{
					"a1": {
						Id: &labapi.Dut_Id{Value: androidDUTID},
					},
				},
				DevboardDUTLabConfig: map[string]*labapi.Dut{
					"cd1": {
						Id: &labapi.Dut_Id{Value: devboardDUTID},
					},
				},
			},
		},
		&out, testing.NewEntityCondition())
	s := root.NewTestState()

	chromeOSDUT, err := s.ChromeOSDUTLabConfig("")
	if err != nil {
		t.Fatal("Failed to get ChromeOS DUT lab config: ", err)
	}
	if chromeOSDUT.GetId().GetValue() != chromeOSDUTID {
		t.Errorf("Failed to get expected ChromeOS DUT ID: got (%s) wanted(%s)",
			chromeOSDUT.GetId().GetValue(), chromeOSDUTID)
	}
	androidDUT, err := s.AndroidDUTLabConfig("a1")
	if err != nil {
		t.Fatal("Failed to get Android DUT lab config: ", err)
	}
	if androidDUT.GetId().GetValue() != androidDUTID {
		t.Errorf("Failed to get expected Android DUT ID: got (%s) wanted(%s)",
			androidDUT.GetId().GetValue(), androidDUTID)
	}
	devboardDUT, err := s.DevboardDUTLabConfig("cd1")
	if err != nil {
		t.Fatal("Failed to get Devboard DUT lab config: ", err)
	}
	if devboardDUT.GetId().GetValue() != devboardDUTID {
		t.Errorf("Failed to get expected devboard DUT ID: got (%s) wanted(%s)",
			devboardDUT.GetId().GetValue(), devboardDUTID)
	}
}

func TestStateExports(t *gotesting.T) {
	for _, tc := range []struct {
		state   interface{}
		methods []string
	}{
		{
			testing.State{},
			[]string{
				"AndroidDUTLabConfig",
				"AttachErrorHandlers",
				"ChromeOSDUTLabConfig",
				"CloudStorage",
				"CompanionDUT",
				"CompanionDUTs",
				"DUT",
				"DataFileSystem",
				"DataPath",
				"DataPaths",
				"DevboardDUTLabConfig",
				"Error",
				"Errorf",
				"Fatal",
				"Fatalf",
				"Features",
				"FixtFillValue",
				"FixtValue",
				"HasError",
				"KeyDir",
				"KeyFile",
				"Log",
				"Logf",
				"Meta",
				"OutDir",
				"Param",
				"PreValue",
				"PushedFilesToDUT",
				"RPCHint",
				"RequiredVar",
				"Run",
				"ServiceDeps",
				"SoftwareDeps",
				"TestName",
				"VLog",
				"VLogf",
				"Var",
			},
		},
		{
			testing.PreState{},
			[]string{
				"AndroidDUTLabConfig",
				"AttachErrorHandlers",
				"ChromeOSDUTLabConfig",
				"CloudStorage",
				"CompanionDUT",
				"CompanionDUTs",
				"DUT",
				"DataFileSystem",
				"DataPath",
				"DataPaths",
				"DevboardDUTLabConfig",
				"Error",
				"Errorf",
				"Fatal",
				"Fatalf",
				"Features",
				"HasError",
				"KeyDir",
				"KeyFile",
				"Log",
				"Logf",
				"OutDir",
				"PreCtx",
				"PushedFilesToDUT",
				"RPCHint",
				"RequiredVar",
				"ServiceDeps",
				"SoftwareDeps",
				"TestName",
				"VLog",
				"VLogf",
				"Var",
			},
		},
		{
			testing.TestHookState{},
			[]string{
				"AndroidDUTLabConfig",
				"AttachErrorHandlers",
				"ChromeOSDUTLabConfig",
				"CloudStorage",
				"CompanionDUT",
				"CompanionDUTRoles",
				"CompanionDUTs",
				"DUT",
				"DataFileSystem",
				"DataPath",
				"DataPaths",
				"DevboardDUTLabConfig",
				"Error",
				"Errorf",
				"Fatal",
				"Fatalf",
				"Features",
				"HasError",
				"KeyDir",
				"KeyFile",
				"Log",
				"Logf",
				"MaxSysMsgLogSize",
				"OutDir",
				"Purgeable",
				"PushedFilesToDUT",
				"RPCHint",
				"RequiredVar",
				"ServiceDeps",
				"SoftwareDeps",
				"TestName",
				"VLog",
				"VLogf",
				"Var",
			},
		},
		{
			testing.FixtState{},
			[]string{
				"AndroidDUTLabConfig",
				"AttachErrorHandlers",
				"ChromeOSDUTLabConfig",
				"CloudStorage",
				"CompanionDUT",
				"CompanionDUTs",
				"DUT",
				"DataFileSystem",
				"DataPath",
				"DataPaths",
				"DevboardDUTLabConfig",
				"Error",
				"Errorf",
				"Fatal",
				"Fatalf",
				"Features",
				"FixtContext",
				"HasError",
				"KeyDir",
				"KeyFile",
				"Log",
				"Logf",
				"OutDir",
				"Param",
				"ParentFillValue",
				"ParentValue",
				"PushedFilesToDUT",
				"RPCHint",
				"RequiredVar",
				"VLog",
				"VLogf",
				"Var",
				// TODO(crbug.com/1035940): Provide access to services.
			},
		},
		{
			testing.FixtTestState{},
			[]string{
				"AndroidDUTLabConfig",
				"AttachErrorHandlers",
				"ChromeOSDUTLabConfig",
				"CloudStorage",
				"CompanionDUT",
				"CompanionDUTs",
				"DUT",
				"DevboardDUTLabConfig",
				"Error",
				"Errorf",
				"Fatal",
				"Fatalf",
				"Features",
				"HasError",
				"KeyDir",
				"KeyFile",
				"Log",
				"Logf",
				"OutDir",
				"PushedFilesToDUT",
				"RPCHint",
				"TestContext",
				"TestName",
				"VLog",
				"VLogf",
			},
		},
	} {
		tv := reflect.TypeOf(tc.state)
		t.Run(tv.Name(), func(t *gotesting.T) {
			// Check that no public field is exported.
			for i := 0; i < tv.NumField(); i++ {
				name := tv.Field(i).Name
				if token.IsExported(name) {
					t.Errorf("Field %s is exposed", name)
				}
			}

			// Check that expected methods are exported.
			tp := reflect.PtrTo(tv)
			var methods []string
			for i := 0; i < tp.NumMethod(); i++ {
				methods = append(methods, tp.Method(i).Name)
			}
			if diff := cmp.Diff(methods, tc.methods); diff != "" {
				t.Errorf("Methods unmatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestHardwareFeatures(t *gotesting.T) {
	features := map[string]*frameworkprotocol.DUTFeatures{
		"": {
			Hardware: &frameworkprotocol.HardwareFeatures{
				HardwareFeatures: &api.HardwareFeatures{
					Screen: &api.HardwareFeatures_Screen{
						PanelProperties: &api.Component_DisplayPanel_Properties{},
					},
				},
			},
		},

		"cd1": {
			Hardware: &frameworkprotocol.HardwareFeatures{
				HardwareFeatures: &api.HardwareFeatures{
					Storage: &api.HardwareFeatures_Storage{
						StorageType: api.Component_Storage_NVME,
					},
				},
			},
		},
	}
	test := &testing.TestInstance{}
	cfg := &testing.RuntimeConfig{Features: features}
	var out outputSink
	root := testing.NewTestEntityRoot(test, cfg, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	for role, features := range features {
		got := s.Features(role)

		if !proto.Equal(got, features) {
			t.Errorf("Failed to get hardware feature for %q (-got +want):\n%s\n%s", role, got, features)
		}
	}
}

func TestOnError(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	errorMsg := "s.Error is called"
	errorfMsg := "s.Errorf is called"

	callCount := 0
	onErrorForErrorCalled := false
	onErrorForErrorfCalled := false
	onError := func(errMsg string) {
		if errMsg == errorMsg {
			onErrorForErrorCalled = true
		}
		callCount++
	}
	s.AttachErrorHandlers(onError, nil)
	onErrorf := func(errMsg string) {
		if errMsg == errorfMsg {
			onErrorForErrorfCalled = true
		}
		callCount++
	}
	s.AttachErrorHandlers(onErrorf, nil)

	s.Error(errorMsg)
	s.Errorf("%s", errorfMsg)

	if !onErrorForErrorCalled {
		t.Error("Failed to get onError callback when s.Error is called")
	}
	if !onErrorForErrorfCalled {
		t.Error("Failed to get onError callback when s.Errorf is called")
	}
	if callCount != 4 {
		t.Errorf("Failled to get correct callback cout got %d; wanted 4", callCount)
	}
}

func TestOnFatalForFatal(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	onFatalCalled := false
	onFatal := func(errMsg string) { onFatalCalled = true }

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	go func() {
		defer close(done)
		s.AttachErrorHandlers(nil, onFatal)
		s.Fatal("fatal")
	}()
	<-done

	if !onFatalCalled {
		t.Errorf("Failed to get onFatal callback when s.Fatal is called")
	}
}

func TestOnFatalForFatalf(t *gotesting.T) {
	var out outputSink
	root := testing.NewTestEntityRoot(&testing.TestInstance{Timeout: time.Minute}, &testing.RuntimeConfig{}, &out, testing.NewEntityCondition())
	s := root.NewTestState()

	onFatalCalled := false
	onFatal := func(errMsg string) { onFatalCalled = true }

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	go func() {
		defer close(done)
		s.AttachErrorHandlers(nil, onFatal)
		s.Fatalf("fatal %s", "msg")
	}()
	<-done

	if !onFatalCalled {
		t.Errorf("Failed to get onFatal callback when s.Fatalf is called")
	}
}
