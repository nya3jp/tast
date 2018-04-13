// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"reflect"
	gotesting "testing"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"
	"chromiumos/tast/testing"

	"github.com/google/subcommands"
)

func init() {
	lg = logging.NewSimple(os.Stdout, log.LstdFlags, true)
}

// executeRunCmd creates a runCmd and executes it using the supplied args and wrapper.
func executeRunCmd(t *gotesting.T, args []string, wrapper *stubRunWrapper) subcommands.ExitStatus {
	cmd := runCmd{wrapper: wrapper}
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	status := cmd.Execute(context.Background(), flags)

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
	executeRunCmd(t, args, &wrapper)
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
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}
}

func TestRunResults(t *gotesting.T) {
	// As long as results were returned, success should be reported.
	wrapper := stubRunWrapper{runRes: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}}}
	args := []string{"root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if !reflect.DeepEqual(wrapper.writeRes, wrapper.runRes) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.writeRes, wrapper.runRes)
	}
}

func TestRunExecFailure(t *gotesting.T) {
	// If tests fail to be executed, an error should be reported.
	wrapper := stubRunWrapper{
		runRes:    []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		runStatus: subcommands.ExitFailure,
	}
	args := []string{"root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != wrapper.runStatus {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, wrapper.runStatus)
	}
	// The partial results should still be written.
	if !reflect.DeepEqual(wrapper.writeRes, wrapper.runRes) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.writeRes, wrapper.runRes)
	}
}

func TestRunWriteFailure(t *gotesting.T) {
	// If writing results fails, an error should be reported.
	wrapper := stubRunWrapper{
		runRes:   []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		writeErr: errors.New("writing failed"),
	}
	args := []string{"root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}
}
