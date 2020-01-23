// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"io/ioutil"
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

	"chromiumos/tast/errors"
	"chromiumos/tast/testutil"
)

func TestLog(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})
	s.Log("msg ", 1)
	s.Logf("msg %d", 2)
	close(or.ch)
	out := or.read()
	if len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
	}
}

func TestNestedRun(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})
	ctx := context.Background()

	s.Run(ctx, "p1", func(ctx context.Context, s *State) {
		s.Log("msg ", 1)

		s.Run(ctx, "p2", func(ctx context.Context, s *State) {
			s.Log("msg ", 2)
		})

		s.Log("msg ", 3)
	})

	s.Log("msg ", 4)

	close(or.ch)
	out := or.read()
	if len(out) != 6 || out[0].Msg != "Starting subtest p1" || out[1].Msg != "msg 1" || out[2].Msg != "Starting subtest p1/p2" ||
		out[3].Msg != "msg 2" || out[4].Msg != "msg 3" || out[5].Msg != "msg 4" {

		t.Errorf("Bad test output: %v", out)
	}
}

func TestParallelRun(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	s.Run(ctx, "r", func(ctx context.Context, s *State) {
		go func() {
			s.Run(ctx, "t1", func(ctx context.Context, s *State) {
				s.Log("msg ", 1)
			})
			wg.Done()
		}()
		go func() {
			s.Run(ctx, "t2", func(ctx context.Context, s *State) {
				s.Log("msg ", 2)
			})
			wg.Done()
		}()
	})
	wg.Wait()

	close(or.ch)
	out := or.read()
	if len(out) != 5 || out[0].Msg != "Starting subtest r" {
		t.Fatal("Bad test output: ", out)
	}

	// Check that both messages appear and are sequential. Ordering between
	// subtests is random.

	hasOutput := func(id string) bool {
		var relatedLogs []string
		for _, log := range out[1:] {
			if strings.HasSuffix(log.Msg, id) {
				relatedLogs = append(relatedLogs, log.Msg)
			}
		}
		return len(relatedLogs) == 2 &&
			strings.HasPrefix(relatedLogs[0], "Starting subtest") &&
			strings.HasPrefix(relatedLogs[1], "msg")
	}

	if !hasOutput("1") || !hasOutput("2") {
		t.Error("Bad test output: ", out)
	}
}

func TestReportError(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})

	// Keep these lines next to each other (see below comparison).
	s.Error("error ", 1)
	s.Errorf("error %d", 2)
	close(or.ch)

	out := or.read()
	if len(out) != 2 {
		t.Fatalf("Got %v output(s); want 2", len(out))
	}

	e0, e1 := out[0].Err, out[1].Err
	if e0 == nil || e1 == nil {
		t.Fatal("Got nil error(s)")
	}
	if act, exp := []string{e0.Reason, e1.Reason}, []string{"error 1", "error 2"}; !reflect.DeepEqual(act, exp) {
		t.Errorf("Got reasons %v; want %v", act, exp)
	}
	if _, fn, _, _ := runtime.Caller(0); e0.File != fn || e1.File != fn {
		t.Errorf("Got filenames %q and %q; want %q", e0.File, e1.File, fn)
	}
	if e0.Line+1 != e1.Line {
		t.Errorf("Got non-sequential line numbers %d and %d", e0.Line, e1.Line)
	}

	for _, e := range []*Error{e0, e1} {
		lines := strings.Split(string(e.Stack), "\n")
		if len(lines) < 2 {
			t.Errorf("Stack trace %q contains fewer than 2 lines", string(e.Stack))
			continue
		}
		if exp := "error "; !strings.HasPrefix(lines[0], exp) {
			t.Errorf("First line of stack trace %q doesn't start with %q", string(e.Stack), exp)
		}
		if exp := fmt.Sprintf("\tat chromiumos/tast/testing.TestReportError (%s:%d)", filepath.Base(e.File), e.Line); lines[1] != exp {
			t.Errorf("Second line of stack trace %q doesn't match %q", string(e.Stack), exp)
		}
	}
}

func TestReportErrorInPrecondition(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})
	s.inPre = true

	// Keep these lines next to each other (see below comparison).
	s.Error("error ", 1)
	s.Errorf("error %d", 2)
	close(or.ch)

	out := or.read()
	if len(out) != 2 {
		t.Fatalf("Got %v output(s); want 2", len(out))
	}

	e0, e1 := out[0].Err, out[1].Err
	if e0 == nil || e1 == nil {
		t.Fatal("Got nil error(s)")
	}
	if act, exp := []string{e0.Reason, e1.Reason}, []string{preFailPrefix + "error 1", preFailPrefix + "error 2"}; !reflect.DeepEqual(act, exp) {
		t.Errorf("Got reasons %v; want %v", act, exp)
	}
	if _, fn, _, _ := runtime.Caller(0); e0.File != fn || e1.File != fn {
		t.Errorf("Got filenames %q and %q; want %q", e0.File, e1.File, fn)
	}
	if e0.Line+1 != e1.Line {
		t.Errorf("Got non-sequential line numbers %d and %d", e0.Line, e1.Line)
	}

	for _, e := range []*Error{e0, e1} {
		lines := strings.Split(string(e.Stack), "\n")
		if len(lines) < 2 {
			t.Errorf("Stack trace %q contains fewer than 2 lines", string(e.Stack))
			continue
		}
		if exp := preFailPrefix + "error "; !strings.HasPrefix(lines[0], exp) {
			t.Errorf("First line of stack trace %q doesn't start with %q", string(e.Stack), exp)
		}
		if exp := fmt.Sprintf("\tat chromiumos/tast/testing.TestReportErrorInPre (%s:%d)", filepath.Base(e.File), e.Line); lines[1] != exp {
			t.Errorf("Second line of stack trace %q doesn't match %q", string(e.Stack), exp)
		}
	}
}

func errorFunc() error {
	return errors.New("meow")
}

func TestExtractErrorSimple(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})

	err := errorFunc()
	s.Error(err)
	close(or.ch)

	out := or.read()
	if len(out) != 1 {
		t.Fatalf("Got %v output(s); want 1", len(out))
	}

	e := out[0].Err

	if exp := "meow"; e.Reason != exp {
		t.Errorf("Error message %q is not %q", e.Reason, exp)
	}
	if exp := "meow\n\tat chromiumos/tast/testing.TestExtractErrorSimple"; !strings.HasPrefix(e.Stack, exp) {
		t.Errorf("Stack trace %q doesn't start with %q", e.Stack, exp)
	}
	if exp := "meow\n\tat chromiumos/tast/testing.errorFunc"; !strings.Contains(e.Stack, exp) {
		t.Errorf("Stack trace %q doesn't contain %q", e.Stack, exp)
	}
}

func TestExtractErrorHeuristic(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})

	err := errorFunc()
	s.Error("Failed something  :  ", err)
	s.Error("Failed something  ", err)
	s.Errorf("Failed something  :  %v", err)
	s.Errorf("Failed something  %v", err)
	close(or.ch)

	out := or.read()
	if len(out) != 4 {
		t.Fatalf("Got %v output(s); want 4", len(out))
	}

	for _, o := range out {
		e := o.Err
		if exp := "Failed something  "; !strings.HasPrefix(e.Reason, exp) {
			t.Errorf("Error message %q doesn't start with %q", e.Reason, exp)
		}
		if exp := "Failed something\n\tat chromiumos/tast/testing.TestExtractErrorHeuristic"; !strings.HasPrefix(e.Stack, exp) {
			t.Errorf("Stack trace %q doesn't start with %q", e.Stack, exp)
		}
		if exp := "\nmeow\n\tat chromiumos/tast/testing.errorFunc"; !strings.Contains(e.Stack, exp) {
			t.Errorf("Stack trace %q doesn't contain %q", e.Stack, exp)
		}
	}
}

func TestRunUsePrefix(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})

	ctx := context.Background()
	s.Run(ctx, "f1", func(ctx context.Context, s *State) {
		s.Run(ctx, "f2", func(ctx context.Context, s *State) {
			s.Errorf("error %s", "msg")
		})
	})
	close(or.ch)

	if !s.HasError() {
		t.Error("Test is not reporting error")
	}

	if out := or.read(); len(out) != 3 {
		t.Errorf("Got %v outputs; want 3", len(out))
	} else {
		if out[0].Err != nil || out[0].Msg != "Starting subtest f1" {
			t.Errorf("Got output %v; want msg %q", out[0].Msg, "Starting subtest f1")
		}

		if out[1].Err != nil || out[1].Msg != "Starting subtest f1/f2" {
			t.Errorf("Got output %v; want msg %q", out[1].Msg, "Starting subtest f1/f2")
		}

		if out[2].Err == nil || out[2].Err.Reason != "f1/f2: error msg" {
			t.Errorf("Got output %v; want reason %q", out[2].Err, "f1/f2: error msg")
		}
	}
}

func TestRunNonFatal(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer func() {
			close(done)
			close(or.ch)
		}()

		ctx := context.Background()
		s.Run(ctx, "f", func(ctx context.Context, s *State) {
			s.Fatal("fatal msg")
		})

		died = false
	}()
	<-done

	if died {
		t.Error("Test stopped due to fail")
	}
}

func TestFatal(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer func() {
			close(done)
			close(or.ch)
		}()
		s.Fatalf("fatal %s", "msg")
		died = false
	}()
	<-done

	if !died {
		t.Fatal("Test continued after call to Fatalf")
	}
	if out := or.read(); len(out) != 1 {
		t.Errorf("Got %v outputs; want 1", len(out))
	} else if out[0].Err == nil || out[0].Err.Reason != "fatal msg" {
		t.Errorf("Got output %v; want reason %q", out[0].Err, "fatal msg")
	}
}

func TestFatalInPrecondition(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{Timeout: time.Minute}, or.ch, &TestConfig{})
	s.inPre = true

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer func() {
			close(done)
			close(or.ch)
		}()
		s.Fatalf("fatal %s", "msg")
		died = false
	}()
	<-done

	if !died {
		t.Fatal("Test continued after call to Fatalf")
	}
	if out := or.read(); len(out) != 1 {
		t.Errorf("Got %v outputs; want 1", len(out))
	} else if out[0].Err == nil || out[0].Err.Reason != preFailPrefix+"fatal msg" {
		t.Errorf("Got output %v; want reason %q", out[0].Err, preFailPrefix+"fatal msg")
	}
}

func TestDataPathDeclared(t *gotesting.T) {
	const (
		dataDir = "/tmp/data"
	)
	test := TestInstance{
		Timeout: time.Minute,
		Data:    []string{"foo", "foo/bar", "foo/baz"},
	}

	for _, tc := range []struct{ in, exp string }{
		{"foo", filepath.Join(dataDir, "foo")},
		{"foo/bar", filepath.Join(dataDir, "foo/bar")},
	} {
		or := newOutputReader()
		s := newState(&test, or.ch, &TestConfig{DataDir: dataDir})
		if act := s.DataPath(tc.in); act != tc.exp {
			t.Errorf("DataPath(%q) = %q; want %q", tc.in, act, tc.exp)
		}
	}
}

func TestDataPathNotDeclared(t *gotesting.T) {
	or := newOutputReader()
	test := TestInstance{
		Timeout: time.Minute,
		Data:    []string{"foo"},
	}
	s := newState(&test, or.ch, &TestConfig{DataDir: "/data"})

	// Request an undeclared data path to cause a fatal error. Do this in a goroutine
	// so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	go func() {
		defer func() {
			close(done)
			close(or.ch)
		}()
		s.DataPath("bar")
	}()
	<-done

	out := or.read()
	if len(out) != 1 || out[0].Err == nil {
		t.Errorf("Got %v when requesting undeclared data path; wanted 1 error", out)
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

	test := TestInstance{Data: []string{file1}}
	or := newOutputReader()
	s := newState(&test, or.ch, &TestConfig{DataDir: td})

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
		body, err := ioutil.ReadAll(resp.Body)
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
	if !s.HasError() {
		t.Error("State doesn't contain error after requesting unregistered file")
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

	test := &TestInstance{Vars: []string{validName, unsetName}}
	cfg := &TestConfig{Vars: map[string]string{validName: validValue, unregName: unregValue}}
	or := newOutputReader()
	s := newState(test, or.ch, cfg)
	defer close(or.ch)

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
	meta := Meta{TastPath: "/foo/bar", Target: "example.net", RunFlags: []string{"-foo", "-bar"}}
	getMeta := func(test *TestInstance, cfg *TestConfig) (*State, *Meta) {
		or := newOutputReader()
		s := newState(test, or.ch, cfg)

		// Meta can call Fatal, which results in a call to runtime.Goexit(),
		// so run this in a goroutine to isolate it from the test.
		mch := make(chan *Meta)
		go func() {
			var meta *Meta
			defer func() { mch <- meta }()
			meta = s.Meta()
		}()
		return s, <-mch
	}

	const (
		metaTest    = "meta.Test"
		nonMetaTest = "pkg.Test"
	)

	// Meta info should be provided to tests in the "meta" package.
	if s, m := getMeta(&TestInstance{Name: metaTest}, &TestConfig{RemoteData: &RemoteData{Meta: &meta}}); s.HasError() {
		t.Errorf("Meta() reported error for %v", metaTest)
	} else if m == nil {
		t.Errorf("Meta() = nil for %v", metaTest)
	} else if !reflect.DeepEqual(*m, meta) {
		t.Errorf("Meta() = %+v for %v; want %+v", *m, metaTest, meta)
	}

	// Tests not in the "meta" package shouldn't have access to meta info.
	if s, m := getMeta(&TestInstance{Name: nonMetaTest}, &TestConfig{RemoteData: &RemoteData{Meta: &meta}}); !s.HasError() {
		t.Errorf("Meta() didn't report error for %v", nonMetaTest)
	} else if m != nil {
		t.Errorf("Meta() = %+v for %v", *m, nonMetaTest)
	}

	// Check that newState doesn't crash or somehow get a non-nil Meta struct when initially passed a nil struct.
	if s, m := getMeta(&TestInstance{Name: metaTest}, &TestConfig{}); !s.HasError() {
		t.Error("Meta() didn't report error for nil info")
	} else if m != nil {
		t.Errorf("Meta() = %+v despite nil info", *m)
	}
}

func TestRPCHint(t *gotesting.T) {
	hint := RPCHint{LocalBundleDir: "/path/to/bundles"}
	getHint := func(test *TestInstance, cfg *TestConfig) (*State, *RPCHint) {
		or := newOutputReader()
		s := newState(test, or.ch, cfg)

		// RPCHint can call Fatal, which results in a call to runtime.Goexit(),
		// so run this in a goroutine to isolate it from the test.
		mch := make(chan *RPCHint)
		go func() {
			var hint *RPCHint
			defer func() { mch <- hint }()
			hint = s.RPCHint()
		}()
		return s, <-mch
	}

	const (
		remoteTest = "do.Remotely"
		localTest  = "do.Locally"
	)

	// RPCHint should be provided to remote tests.
	if s, h := getHint(&TestInstance{Name: remoteTest}, &TestConfig{RemoteData: &RemoteData{RPCHint: &hint}}); s.HasError() {
		t.Errorf("RPCHint() reported error for %v", remoteTest)
	} else if h == nil {
		t.Errorf("RPCHint() = nil for %v", remoteTest)
	} else if !reflect.DeepEqual(*h, hint) {
		t.Errorf("RPCHint() = %+v for %v; want %+v", *h, remoteTest, hint)
	}

	// Local tests shouldn't have access to RPCHint.
	if s, h := getHint(&TestInstance{Name: localTest}, &TestConfig{}); !s.HasError() {
		t.Errorf("RPCHint() didn't report error for %v", localTest)
	} else if h != nil {
		t.Errorf("RPCHint() = %+v for %v", *h, localTest)
	}
}

func TestDUT(t *gotesting.T) {
	callDUT := func(test *TestInstance, cfg *TestConfig) *State {
		or := newOutputReader()
		s := newState(test, or.ch, cfg)

		// DUT can call Fatal, which results in a call to runtime.Goexit(),
		// so run this in a goroutine to isolate it from the test.
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.DUT()
		}()
		<-done
		return s
	}

	const (
		remoteTest = "do.Remotely"
		localTest  = "do.Locally"
	)

	// DUT should be provided to remote tests.
	if s := callDUT(&TestInstance{Name: remoteTest}, &TestConfig{RemoteData: &RemoteData{}}); s.HasError() {
		t.Errorf("DUT() reported error for %v", remoteTest)
	}

	// Local tests shouldn't have access to DUT.
	if s := callDUT(&TestInstance{Name: localTest}, &TestConfig{}); !s.HasError() {
		t.Errorf("DUT() didn't report error for %v", localTest)
	}
}

func TestCloudStorage(t *gotesting.T) {
	want := NewCloudStorage(nil)

	or := newOutputReader()
	s := newState(&TestInstance{Name: "example.Test"}, or.ch, &TestConfig{CloudStorage: want})
	got := s.CloudStorage()

	if got != want {
		t.Errorf("CloudStorage returned %v; want %v", got, want)
	}
}
