// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	gotesting "testing"
	"time"
)

// readOutput reads and returns Output entries from ch.
func readOutput(ch chan Output) []Output {
	res := make([]Output, 0)
	for o := range ch {
		res = append(res, o)
	}
	return res
}

func TestLog(t *gotesting.T) {
	ch := make(chan Output, 2)
	s := NewState(context.Background(), &Test{Timeout: time.Minute}, ch, "", "")
	s.Log("msg ", 1)
	s.Logf("msg %d", 2)
	close(ch)
	out := readOutput(ch)
	if len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
	}
}

func TestReportError(t *gotesting.T) {
	ch := make(chan Output, 2)
	s := NewState(context.Background(), &Test{Timeout: time.Minute}, ch, "", "")

	// Keep these lines next to each other (see below comparison).
	s.Error("error ", 1)
	s.Errorf("error %d", 2)
	close(ch)

	out := readOutput(ch)
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

	// The initial lines of the stack trace should be dropped such that the first frame
	// is the code that called Error or Errorf. As such, the first line should contain this
	// test function's name and the second line should contain the filename and line number
	// of the error call.
	const fc = "testing.TestReportError("
	for _, e := range []*Error{e0, e1} {
		lines := strings.Split(string(e.Stack), "\n")
		if len(lines) < 2 {
			t.Errorf("Stack trace %q contains fewer than 2 lines", string(e.Stack))
			continue
		}
		if !strings.Contains(lines[0], fc) {
			t.Errorf("First line of stack trace %q doesn't contain %q", string(e.Stack), fc)
		}
		fl := fmt.Sprintf("%s:%d", e.File, e.Line)
		if !strings.Contains(lines[1], fl) {
			t.Errorf("Second line of stack trace %q doesn't contain %q", string(e.Stack), fl)
		}
	}
}

func TestFatal(t *gotesting.T) {
	ch := make(chan Output, 2)
	s := NewState(context.Background(), &Test{Timeout: time.Minute}, ch, "", "")

	// Log the fatal message in a goroutine so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	died := true
	go func() {
		defer func() {
			close(done)
			close(ch)
		}()
		s.Fatalf("fatal %s", "msg")
		died = false
	}()
	<-done

	if !died {
		t.Errorf("Test continued after call to Fatalf")
	}
	out := readOutput(ch)
	if len(out) != 1 {
		t.Errorf("Got %v outputs; want 1", len(out))
	} else if out[0].Err == nil || out[0].Err.Reason != "fatal msg" {
		t.Errorf("Got output %v; want reason %q", out[0].Err, "fatal msg")
	}
}

func TestContextLog(t *gotesting.T) {
	ch := make(chan Output, 2)
	s := NewState(context.Background(), &Test{Timeout: time.Minute}, ch, "", "")
	ContextLog(s.Context(), "msg ", 1)
	ContextLogf(s.Context(), "msg %d", 2)
	close(ch)
	out := readOutput(ch)
	if len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
	}
}

func TestDataPathValid(t *gotesting.T) {
	const (
		dataDir = "/tmp/data"
	)

	for _, tc := range []struct{ in, exp string }{
		{"foo", filepath.Join(dataDir, "foo")},
		{"foo/bar", filepath.Join(dataDir, "foo/bar")},
	} {
		s := NewState(context.Background(), &Test{Timeout: time.Minute}, make(chan Output), dataDir, "")
		if act := s.DataPath(tc.in); act != tc.exp {
			t.Errorf("DataPath(%q) = %q; want %q", tc.in, act, tc.exp)
		}
	}
}

func TestDataPathNoParent(t *gotesting.T) {
	ch := make(chan Output, 1)
	s := NewState(context.Background(), &Test{Timeout: time.Minute}, ch, "/data", "")

	// Request an invalid data path to cause a fatal error. Do this in a goroutine
	// so the main goroutine that's running the test won't exit.
	done := make(chan bool)
	go func() {
		defer func() {
			close(done)
			close(ch)
		}()
		s.DataPath("../foo.bin")
	}()
	<-done

	out := readOutput(ch)
	if len(out) != 1 || out[0].Err == nil {
		t.Errorf("Got %v when requesting data path containing '..'; wanted 1 error", out)
	}
}
