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

	"chromiumos/tast/cmd/tast/logging"
	"chromiumos/tast/cmd/tast/run"
	"chromiumos/tast/command"
	"chromiumos/tast/testing"
)

// listCmd implements subcommands.Command to support listing tests.
type listCmd struct {
	json       bool        // marshal tests to JSON instead of just printing names
	inputFiles []string    // files containing JSON arrays of testing.Test structs to use instead of querying DUT
	cfg        *run.Config // shared config for listing tests
	wrapper    runWrapper  // wraps calls to run package
	stdout     io.Writer   // where to write tests
}

var _ = subcommands.Command(&runCmd{})

// newListCmd returns a new listCmd that will write tests to stdout.
func newListCmd(stdout io.Writer, trunkDir string) *listCmd {
	return &listCmd{
		cfg:     run.NewConfig(run.ListTestsMode, tastDir, trunkDir),
		wrapper: &realRunWrapper{},
		stdout:  stdout,
	}
}

func (*listCmd) Name() string     { return "list" }
func (*listCmd) Synopsis() string { return "list tests" }
func (*listCmd) Usage() string {
	return `Usage: list [flag]... <target> [pattern]...

List tests matched by zero or more patterns.

The target is an SSH connection spec of the form "[user@]host[:port]".
It is omitted if -readfile is passed.

Patterns are either globs matching test names or a single test attribute
boolean expression in parentheses (e.g. "(informational && !disabled)").

`
}

func (lc *listCmd) SetFlags(f *flag.FlagSet) {
	// TODO(derat): Add -listtype: https://crbug.com/831849
	f.BoolVar(&lc.json, "json", false, "print full test details as JSON")

	rf := command.RepeatedFlag(func(v string) error {
		lc.inputFiles = append(lc.inputFiles, v)
		return nil
	})
	f.Var(&rf, "readfile", "read JSON array of testing.Test structs from file "+
		"instead of querying DUT (may be repeated)")

	lc.cfg.SetFlags(f)
}

func (lc *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

	var tests []*testing.TestCase

	if len(lc.inputFiles) > 0 {
		reg := testing.NewRegistry()
		for _, p := range lc.inputFiles {
			if err := importTests(p, reg); err != nil {
				lg.Logf("Failed to import tests from %v: %v", p, err)
				return subcommands.ExitFailure
			}
		}
		var err error
		if tests, err = testing.SelectTestsByArgs(reg.AllTests(), f.Args()); err != nil {
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
		tests = make([]*testing.TestCase, len(results))
		for i := range results {
			tests[i] = &results[i].TestCase
		}
	}

	if err := lc.printTests(tests); err != nil {
		lg.Log("Failed to write tests: ", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// printTests writes the supplied tests to lc.stdout.
func (lc *listCmd) printTests(tests []*testing.TestCase) error {
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

// importTests unmarshals a JSON array of testing.Test structs from the file at p and imports them into reg.
// The returned tests will not be runnable, as test functions are not preserved during marshaling.
func importTests(p string, reg *testing.Registry) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()

	var ts []*testing.TestCase
	if err := json.NewDecoder(f).Decode(&ts); err != nil {
		return err
	}
	for _, t := range ts {
		if err := reg.AddTestCase(t); err != nil {
			return fmt.Errorf("failed to add test %v: %v", t.Name, err)
		}
	}
	return nil
}
