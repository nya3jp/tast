// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/testutil"
)

func TestLog(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&Test{Timeout: time.Minute}, or.ch, &TestConfig{})
	s.Log("msg ", 1)
	s.Logf("msg %d", 2)
	close(or.ch)
	out := or.read()
	if len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
	}
}

func TestReportError(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&Test{Timeout: time.Minute}, or.ch, &TestConfig{})

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

func errorFunc() error {
	return errors.New("meow")
}

func TestExtractErrorSimple(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&Test{Timeout: time.Minute}, or.ch, &TestConfig{})

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
	s := newState(&Test{Timeout: time.Minute}, or.ch, &TestConfig{})

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

func TestFatal(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&Test{Timeout: time.Minute}, or.ch, &TestConfig{})

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
		t.Errorf("Test continued after call to Fatalf")
	}
	if out := or.read(); len(out) != 1 {
		t.Errorf("Got %v outputs; want 1", len(out))
	} else if out[0].Err == nil || out[0].Err.Reason != "fatal msg" {
		t.Errorf("Got output %v; want reason %q", out[0].Err, "fatal msg")
	}
}

func TestDataPathDeclared(t *gotesting.T) {
	const (
		dataDir = "/tmp/data"
	)
	test := Test{
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
	test := Test{
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

	test := Test{Data: []string{file1}}
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

	test := &Test{Vars: []string{validName, unsetName}}
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
	getMeta := func(test *Test, cfg *TestConfig) (*State, *Meta) {
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
	if s, m := getMeta(&Test{Name: metaTest}, &TestConfig{Meta: &meta}); s.HasError() {
		t.Errorf("Meta() reported error for %v", metaTest)
	} else if m == nil {
		t.Errorf("Meta() = nil for %v", metaTest)
	} else if !reflect.DeepEqual(*m, meta) {
		t.Errorf("Meta() = %+v for %v; want %+v", *m, metaTest, meta)
	}

	// Tests not in the "meta" package shouldn't have access to meta info.
	if s, m := getMeta(&Test{Name: nonMetaTest}, &TestConfig{Meta: &meta}); !s.HasError() {
		t.Errorf("Meta() didn't report error for %v", nonMetaTest)
	} else if m != nil {
		t.Errorf("Meta() = %+v for %v", *m, nonMetaTest)
	}

	// Check that newState doesn't crash or somehow get a non-nil Meta struct when initially passed a nil struct.
	if s, m := getMeta(&Test{Name: metaTest}, &TestConfig{}); !s.HasError() {
		t.Error("Meta() didn't report error for nil info")
	} else if m != nil {
		t.Errorf("Meta() = %+v despite nil info", *m)
	}
}
