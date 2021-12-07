// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"os"
	"reflect"
	"strings"
	gotesting "testing"

	"github.com/google/subcommands"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/run/resultsjson"
	"chromiumos/tast/testutil"
)

// executeRunCmd creates a runCmd and executes it using the supplied args, wrapper, and Logger.
func executeRunCmd(t *gotesting.T, args []string, wrapper *stubRunWrapper, logger logging.Logger) subcommands.ExitStatus {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cmd := newRunCmd(td)
	cmd.wrapper = wrapper
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	flags.Set("build", "false") // DeriveDefaults fails if -build=true and bundle dirs are missing

	ctx := context.Background()
	if logger != nil {
		ctx = logging.AttachLogger(ctx, logger)
	}
	status := cmd.Execute(ctx, flags)

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
	wrapper := stubRunWrapper{runRes: []*resultsjson.Result{}}
	executeRunCmd(t, args, &wrapper, nil)
	if wrapper.runCfg.Target() != target {
		t.Errorf("runCmd.Execute(%v) passed target %q; want %q", args, wrapper.runCfg.Target(), target)
	}
	if exp := []string{test1, test2}; !reflect.DeepEqual(wrapper.runCfg.Patterns(), exp) {
		t.Errorf("runCmd.Execute(%v) passed patterns %v; want %v", args, wrapper.runCfg.Patterns(), exp)
	}
}

func TestRunNoResults(t *gotesting.T) {
	// The run should fail if no tests were matched.
	args := []string{"root@example.net"}
	wrapper := stubRunWrapper{runRes: []*resultsjson.Result{}}
	if status := executeRunCmd(t, args, &wrapper, nil); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}
}

func TestRunResults(t *gotesting.T) {
	// As long as results were returned and no run-level errors occurred, success should be reported.
	wrapper := stubRunWrapper{runRes: []*resultsjson.Result{
		{
			Test:   resultsjson.Test{Name: "pkg.LocalTest"},
			Errors: []resultsjson.Error{{}},
		},
	}}
	args := []string{"root@example.net"}
	if status := executeRunCmd(t, args, &wrapper, nil); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}

	// If -failfortests is passed, then a test failure should result in 1 being returned.
	args = append([]string{"-failfortests"}, args...)
	if status := executeRunCmd(t, args, &wrapper, nil); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v for failing test; want %v", args, status, subcommands.ExitFailure)
	}

	// If the test passed, we should return 0 with -failfortests.
	wrapper.runRes[0].Errors = nil
	if status := executeRunCmd(t, args, &wrapper, nil); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v for successful test; want %v", args, status, subcommands.ExitSuccess)
	}
}

func TestRunExecFailure(t *gotesting.T) {
	// If tests fail to be executed, an error should be reported.
	const msg = "exec failed"
	wrapper := stubRunWrapper{
		runRes: []*resultsjson.Result{{Test: resultsjson.Test{Name: "pkg.LocalTest"}}},
		runErr: errors.New(msg),
	}
	args := []string{"root@example.net"}
	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	if status := executeRunCmd(t, args, &wrapper, logger); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}

	// The error message should be in the last line of output.
	lines := logger.Logs()
	if len(lines) == 0 {
		t.Errorf("runCmd.Execute(%v) didn't log any output", args)
	} else if last := lines[len(lines)-1]; !strings.Contains(last, msg) {
		t.Errorf("runCmd.Execute(%v) logged last line %q; wanted line containing error %q", args, last, msg)
	}
}
