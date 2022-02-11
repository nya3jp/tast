// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/google/subcommands"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/run/resultsjson"
)

// listCmd implements subcommands.Command to support listing tests.
type listCmd struct {
	json    bool                  // marshal tests to JSON instead of just printing names
	cfg     *config.MutableConfig // shared config for listing tests
	wrapper runWrapper            // wraps calls to run package
	stdout  io.Writer             // where to write tests
}

var _ = subcommands.Command(&runCmd{})

// newListCmd returns a new listCmd that will write tests to stdout.
func newListCmd(stdout io.Writer, trunkDir string) *listCmd {
	return &listCmd{
		cfg:     config.NewMutableConfig(config.ListTestsMode, tastDir, trunkDir),
		wrapper: &realRunWrapper{},
		stdout:  stdout,
	}
}

func (*listCmd) Name() string     { return "list" }
func (*listCmd) Synopsis() string { return "list tests" }
func (*listCmd) Usage() string {
	return `Usage: list [flag]... <target> [pattern]...

Description:
    List tests matched by zero or more patterns.

Target:
    The target is an SSH connection spec of the form "[user@]host[:port]".

Pattern:
    Patterns are either globs matching test names or a single test attribute
    boolean expression in parentheses.

    To list all the tests:
        
        $ tast list <target>             
  
    To list tests based attributes pattern, mention single argument surrounded by parentheses. Example:

        $ tast list <target> '(("dep:chrome" || "dep:android") && !informational)'
    
    To list tests based on test name wildcard pattern, use *. Example:

        $ tast list <target> 'ui*' 'wilco*'

Flag:
`
}

func (lc *listCmd) SetFlags(f *flag.FlagSet) {
	// TODO(derat): Add -listtype: https://crbug.com/831849
	f.BoolVar(&lc.json, "json", false, "print full test details as JSON")
	lc.cfg.SetFlags(f)
}

func (lc *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(f.Args()) == 0 {
		logging.Info(ctx, "Missing target.\n\n"+lc.Usage())
		return subcommands.ExitUsageError
	}
	if err := lc.cfg.DeriveDefaults(); err != nil {
		logging.Info(ctx, "Failed to derive defaults: ", err)
		return subcommands.ExitUsageError
	}
	lc.cfg.Target = f.Args()[0]
	lc.cfg.Patterns = f.Args()[1:]

	logger := logging.NewSinkLogger(logging.LevelDebug, true, logging.NewWriterSink(ioutil.Discard))
	ctx = logging.AttachLoggerNoPropagation(ctx, logger)

	state := config.DeprecatedState{}
	results, err := lc.wrapper.run(ctx, lc.cfg.Freeze(), &state)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return subcommands.ExitFailure
	}
	tests := make([]*resultsjson.Test, len(results))
	for i := range results {
		tests[i] = &results[i].Test
	}

	if err := lc.printTests(tests); err != nil {
		logging.Info(ctx, "Failed to write tests: ", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// printTests writes the supplied tests to lc.stdout.
func (lc *listCmd) printTests(tests []*resultsjson.Test) error {
	if lc.json {
		enc := json.NewEncoder(lc.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tests)
	}

	// If -json wasn't passed, just print test names, one per line.
	for _, t := range tests {
		if _, err := fmt.Fprintln(lc.stdout, t.Name); err != nil {
			return err
		}
	}
	return nil
}
