// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"log"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"
	"chromiumos/tast/testing"

	"github.com/google/subcommands"
)

// executeRunCmd creates a runCmd and executes it using the supplied args, wrapper, and Logger.
func executeRunCmd(t *gotesting.T, args []string, wrapper *stubRunWrapper, lg logging.Logger) subcommands.ExitStatus {
	cmd := newRunCmd()
	cmd.wrapper = wrapper
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	status := cmd.Execute(logging.NewContext(context.Background(), lg), flags)

	if wrapper.runRes != nil && wrapper.runCfg == nil {
		t.Fatalf("runCmd.Execute(%v) unexpectedly didn't run tests", args)
	} else if wrapper.runRes == nil && wrapper.runCfg != nil {
		t.Fatalf("runCmd.Execute(%v) unexpectedly ran tests", args)
	}
	return status
}

func TestRunConfig(t *gotesting.T) {
	const (
		target = "root@example.net"
		test1  = "pkg.Test1"
		test2  = "pkg.Test2"
	)
	args := []string{target, test1, test2}
	wrapper := stubRunWrapper{runRes: []run.TestResult{}}
	executeRunCmd(t, args, &wrapper, logging.NewDiscard())
	if wrapper.runCfg.Target != target {
		t.Errorf("runCmd.Execute(%v) passed target %q; want %q", args, wrapper.runCfg.Target, target)
	}
	if exp := []string{test1, test2}; !reflect.DeepEqual(wrapper.runCfg.Patterns, exp) {
		t.Errorf("runCmd.Execute(%v) passed patterns %v; want %v", args, wrapper.runCfg.Patterns, exp)
	}
}

func TestRunNoResults(t *gotesting.T) {
	// The run should fail if no tests were matched.
	args := []string{"root@example.net"}
	wrapper := stubRunWrapper{runRes: []run.TestResult{}}
	if status := executeRunCmd(t, args, &wrapper, logging.NewDiscard()); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}
}

func TestRunResults(t *gotesting.T) {
	// As long as results were returned, success should be reported.
	wrapper := stubRunWrapper{runRes: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}}}
	args := []string{"root@example.net"}
	if status := executeRunCmd(t, args, &wrapper, logging.NewDiscard()); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if !reflect.DeepEqual(wrapper.writeRes, wrapper.runRes) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.writeRes, wrapper.runRes)
	}
	if !wrapper.writeComplete {
		t.Errorf("runCmd.Execute(%v) reported incomplete run", args)
	}
}

func TestRunExecFailure(t *gotesting.T) {
	// If tests fail to be executed, an error should be reported.
	const msg = "exec failed"
	wrapper := stubRunWrapper{
		runRes:    []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		runStatus: run.Status{ExitCode: subcommands.ExitFailure, ErrorMsg: msg + "\nmore details"},
	}
	args := []string{"root@example.net"}
	b := bytes.Buffer{}
	lg := logging.NewSimple(&b, log.LstdFlags, true)
	if status := executeRunCmd(t, args, &wrapper, lg); status != wrapper.runStatus.ExitCode {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, wrapper.runStatus.ExitCode)
	}

	// The first line of the failure message should be in the last line of output.
	if lines := strings.Split(strings.TrimSpace(b.String()), "\n"); len(lines) == 0 {
		t.Errorf("runCmd.Execute(%v) didn't log any output", args)
	} else if last := lines[len(lines)-1]; !strings.Contains(last, msg) {
		t.Errorf("runCmd.Execute(%v) logged last line %q; wanted line containing error %q", args, last, msg)
	}

	// The partial results should still be written.
	if !reflect.DeepEqual(wrapper.writeRes, wrapper.runRes) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.writeRes, wrapper.runRes)
	}
	if wrapper.writeComplete {
		t.Errorf("runCmd.Execute(%v) reported complete run", args)
	}
}

func TestRunWriteFailure(t *gotesting.T) {
	// If writing results fails, an error should be reported.
	const msg = "writing failed"
	wrapper := stubRunWrapper{
		runRes:   []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		writeErr: errors.New(msg),
	}
	args := []string{"root@example.net"}
	b := bytes.Buffer{}
	lg := logging.NewSimple(&b, log.LstdFlags, true)
	if status := executeRunCmd(t, args, &wrapper, lg); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}

	// The error should be included in the last line of output.
	if lines := strings.Split(strings.TrimSpace(b.String()), "\n"); len(lines) == 0 {
		t.Errorf("runCmd.Execute(%v) didn't log any output", args)
	} else if last := lines[len(lines)-1]; !strings.Contains(last, msg) {
		t.Errorf("runCmd.Execute(%v) logged last line %q; wanted line containing error %q", args, last, msg)
	}
}

func TestRunReserveTimeToWriteResults(t *gotesting.T) {
	wrapper := stubRunWrapper{
		runRes: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.Test"}}},
	}
	executeRunCmd(t, []string{"-timeout=3600", "root@example.net"}, &wrapper, logging.NewDiscard())

	getDeadline := func(ctx context.Context, name string) time.Time {
		if ctx == nil {
			t.Fatalf("%s context not set", name)
		}
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("%s context lacks deadline", name)
		}
		return dl
	}
	rdl := getDeadline(wrapper.runCtx, "run")
	wdl := getDeadline(wrapper.writeCtx, "write")
	if !rdl.Before(wdl) {
		t.Errorf("Run deadline %v doesn't precede results-writing deadline %v", wdl, rdl)
	}
}
