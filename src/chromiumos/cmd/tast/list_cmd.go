// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/google/subcommands"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"
	"chromiumos/tast/bundle"
	"chromiumos/tast/testing"
)

// listCmd implements subcommands.Command to support listing tests.
type listCmd struct {
	json          bool        // marshal tests to JSON instead of just printing names
	readFromStdin bool        // read JSON-marshaled tests from stdin instead of querying DUT
	cfg           *run.Config // shared config for listing tests
	wrapper       runWrapper  // wraps calls to run package
	stdout        io.Writer   // where to write tests
	stdin         io.Reader   // used for reading tests if readFromStdin is true
}

// newListCmd returns a new listCmd that will write tests to stdout.
func newListCmd(stdout io.Writer, stdin io.Reader) *listCmd {
	return &listCmd{
		cfg:     run.NewConfig(run.ListTestsMode, tastDir, trunkDir()),
		wrapper: &realRunWrapper{},
		stdout:  stdout,
		stdin:   stdin,
	}
}

func (*listCmd) Name() string     { return "list" }
func (*listCmd) Synopsis() string { return "list tests" }
func (*listCmd) Usage() string {
	return `Usage: list [flag]... <target> [pattern]...

List tests matched by zero or more patterns.

The target is an SSH connection spec of the form "[user@]host[:port]".
It is omitted if -stdin is passed.

Patterns are either globs matching test names or a single test attribute
boolean expression in parentheses (e.g. "(informational && !disabled)").

`
}

func (lc *listCmd) SetFlags(f *flag.FlagSet) {
	// TODO(derat): Add -listtype: https://crbug.com/831849
	f.BoolVar(&lc.json, "json", false, "print full test details as JSON")
	f.BoolVar(&lc.readFromStdin, "stdin", false, "read JSON arrays of testing.Test structs from stdin instead of querying DUT")
	lc.cfg.SetFlags(f)
}

func (lc *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

	var tests []*testing.Test

	if lc.readFromStdin {
		reg, err := readTestArrays(lc.stdin)
		if err != nil {
			lg.Log("Failed to read tests from stdin: ", err)
			return subcommands.ExitFailure
		}
		if tests, err = bundle.TestsToRun(reg, f.Args()); err != nil {
			lg.Log("Failed to select tests: ", err)
			return subcommands.ExitUsageError
		}
	} else {
		if len(f.Args()) == 0 {
			lg.Log("Missing target.\n\n" + lc.Usage())
			return subcommands.ExitUsageError
		}
		if err := lc.cfg.DeriveDefaults(); err != nil {
			lg.Log("Failed to derive defaults: ", err)
			return subcommands.ExitUsageError
		}
		lc.cfg.Target = f.Args()[0]
		lc.cfg.Patterns = f.Args()[1:]

		b := bytes.Buffer{}
		lc.cfg.Logger = logging.NewSimple(&b, log.LstdFlags, true)

		status, results := lc.wrapper.run(ctx, lc.cfg)
		if status.ExitCode != subcommands.ExitSuccess {
			os.Stderr.Write(b.Bytes())
			return status.ExitCode
		}
		tests = make([]*testing.Test, len(results))
		for i := range tests {
			tests[i] = &results[i].Test
		}
	}

	if err := lc.printTests(tests); err != nil {
		lg.Log("Failed to write tests: ", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// printTests writes the supplied tests to lc.stdout.
func (lc *listCmd) printTests(tests []*testing.Test) error {
	if lc.json {
		enc := json.NewEncoder(lc.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tests)
	}

	// If -json wasn't passed, just print test names, one per line.
	for _, t := range tests {
		if _, err := fmt.Fprintf(lc.stdout, "%s\n", t.Name); err != nil {
			return err
		}
	}
	return nil
}

// readTestArrays reads zero or more JSON arrays of marshaled testing.Test structs
// from r and imports them into a new registry. The returned tests will not be runnable,
// as test functions are not preserved during marshaling.
func readTestArrays(r io.Reader) (*testing.Registry, error) {
	reg := testing.NewRegistry(testing.NoFinalize)
	d := json.NewDecoder(r)
	for {
		var ts []*testing.Test
		if err := d.Decode(&ts); err == io.EOF {
			return reg, nil
		} else if err != nil {
			return nil, fmt.Errorf("failed to decode test array: %v", err)
		}

		for _, t := range ts {
			if err := reg.AddTest(t); err != nil {
				return nil, fmt.Errorf("failed to add test %v: %v", t.Name, err)
			}
		}
	}
}
