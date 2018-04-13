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
// It expects wrapper.local to be called if wrapper.lres is non-nil, and vice versa (and ditto for remote).
func executeRunCmd(t *gotesting.T, args []string, wrapper *stubRunWrapper) subcommands.ExitStatus {
	cmd := runCmd{wrapper: wrapper}
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	status := cmd.Execute(context.Background(), flags)

	if wrapper.lres != nil && wrapper.lcfg == nil {
		t.Fatalf("runCmd.Execute(%v) unexpectedly didn't run local tests", args)
	} else if wrapper.lres == nil && wrapper.lcfg != nil {
		t.Fatalf("runCmd.Execute(%v) unexpectedly ran local tests", args)
	}
	if wrapper.rres != nil && wrapper.rcfg == nil {
		t.Fatalf("runCmd.Execute(%v) unexpectedly didn't run remote tests", args)
	} else if wrapper.rres == nil && wrapper.rcfg != nil {
		t.Fatalf("runCmd.Execute(%v) unexpectedly ran remote tests", args)
	}
	return status
}

func TestRunLocalAndRemote(t *gotesting.T) {
	// With -build=false, both local and remote tests should be executed and written.
	wrapper := stubRunWrapper{
		lres: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		rres: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.RemoteTest"}}},
	}
	args := []string{"-build=false", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if wrapper.lcfg.Build || wrapper.rcfg.Build {
		t.Errorf("runCmd.Execute(%v) requested build in config %v and/or %v", args, wrapper.lcfg, wrapper.rcfg)
	}
	if eres := []run.TestResult{wrapper.lres[0], wrapper.rres[0]}; !reflect.DeepEqual(wrapper.wres, eres) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.wres, eres)
	}
}

func TestRunNoResults(t *gotesting.T) {
	// The run should fail if no local or remote tests were matched.
	args := []string{"-build=false", "root@example.net"}
	wrapper := stubRunWrapper{lres: []run.TestResult{}, rres: []run.TestResult{}}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}
}

func TestRunLocalAndRemotePartialResults(t *gotesting.T) {
	// As long as either local or remote results were returned, success should be reported.
	wrapper := stubRunWrapper{
		lres: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		rres: []run.TestResult{},
	}
	args := []string{"-build=false", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if !reflect.DeepEqual(wrapper.wres, wrapper.lres) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.wres, wrapper.lres)
	}
}

func TestRunLocalFailure(t *gotesting.T) {
	// If local tests fail to be executed, remote tests shouldn't be run.
	wrapper := stubRunWrapper{
		lres:  []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		lstat: subcommands.ExitFailure,
	}
	args := []string{"-build=false", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != wrapper.lstat {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, wrapper.lstat)
	}
	// The partial results should still be written.
	if !reflect.DeepEqual(wrapper.wres, wrapper.lres) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.wres, wrapper.lres)
	}
}

func TestRunRemoteFailure(t *gotesting.T) {
	// If remote tests fail to be executed, we should report overall failure.
	wrapper := stubRunWrapper{
		lres:  []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		rres:  []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.RemoteTest"}}},
		rstat: subcommands.ExitFailure,
	}
	args := []string{"-build=false", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != wrapper.rstat {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, wrapper.rstat)
	}
	// The combined results should still be written.
	if eres := []run.TestResult{wrapper.lres[0], wrapper.rres[0]}; !reflect.DeepEqual(wrapper.wres, eres) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.wres, eres)
	}
}

func TestRunWriteFailure(t *gotesting.T) {
	// If writing results fails, an error should be reported.
	wrapper := stubRunWrapper{
		lres: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}},
		rres: []run.TestResult{},
		werr: errors.New("writing failed"),
	}
	args := []string{"-build=false", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitFailure {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitFailure)
	}
}

func TestRunLocalWithBuild(t *gotesting.T) {
	// Build and run only local tests.
	wrapper := stubRunWrapper{lres: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.LocalTest"}}}}
	args := []string{"-build=true", "-buildtype=local", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if !wrapper.lcfg.Build {
		t.Errorf("runCmd.Execute(%v) didn't request build in config %v", args, wrapper.lcfg)
	}
	if !reflect.DeepEqual(wrapper.wres, wrapper.lres) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.wres, wrapper.lres)
	}
}

func TestRunRemoteWithBuild(t *gotesting.T) {
	// Build and run only remote tests.
	wrapper := stubRunWrapper{rres: []run.TestResult{run.TestResult{Test: testing.Test{Name: "pkg.RemoteTest"}}}}
	args := []string{"-build=true", "-buildtype=remote", "root@example.net"}
	if status := executeRunCmd(t, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("runCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if !wrapper.rcfg.Build {
		t.Errorf("runCmd.Execute(%v) didn't request build in config %v", args, wrapper.rcfg)
	}
	if !reflect.DeepEqual(wrapper.wres, wrapper.rres) {
		t.Errorf("runCmd.Execute(%v) wrote results %v; want %v", args, wrapper.wres, wrapper.rres)
	}
}
