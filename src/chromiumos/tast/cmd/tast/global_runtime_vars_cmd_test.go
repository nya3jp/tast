// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	gotesting "testing"

	"github.com/google/subcommands"

	"chromiumos/tast/testutil"
)

// executeGlobalRuntimeVarsCmd creates a GlobalRuntimeVarsCmd and executes it using the supplied args and wrapper.
func executeGlobalRuntimeVarsCmd(t *gotesting.T, stdout io.Writer, args []string, wrapper *stubRunWrapper) subcommands.ExitStatus {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cmd := newGlobalRuntimeVarsCmd(stdout, td)
	cmd.wrapper = wrapper
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	flags.Set("build", "false") // DeriveDefaults fails if -build=true and bundle dirs are missing
	return cmd.Execute(context.Background(), flags)
}

func TestGlobalRuntimeVars(t *gotesting.T) {
	wrapper := stubRunWrapper{
		runGlobalRuntimeVars: []string{"var1", "var2"},
	}

	// Verify that one-var-per-line works.
	stdout := bytes.Buffer{}
	args := []string{"root@example.net"}
	if status := executeGlobalRuntimeVarsCmd(t, &stdout, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("globalRuntimeVarsCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if exp := fmt.Sprintf("%s\n%s\n", "var1", "var2"); stdout.String() != exp {
		t.Errorf("globalRuntimeVarsCmd.Execute(%v) printed %q; want %q", args, stdout.String(), exp)
	}
}
