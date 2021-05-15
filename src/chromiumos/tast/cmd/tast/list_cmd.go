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
	"os"

	"github.com/google/subcommands"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
)

// listCmd implements subcommands.Command to support listing tests.
type listCmd struct {
	json    bool           // marshal tests to JSON instead of just printing names
	cfg     *config.Config // shared config for listing tests
	wrapper runWrapper     // wraps calls to run package
	stdout  io.Writer      // where to write tests
}

var _ = subcommands.Command(&runCmd{})

// newListCmd returns a new listCmd that will write tests to stdout.
func newListCmd(stdout io.Writer, trunkDir string) *listCmd {
	return &listCmd{
		cfg:     config.NewConfig(config.ListTestsMode, tastDir, trunkDir),
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

Patterns are either globs matching test names or a single test attribute
boolean expression in parentheses (e.g. "(informational && !disabled)").

`
}

func (lc *listCmd) SetFlags(f *flag.FlagSet) {
	// TODO(derat): Add -listtype: https://crbug.com/831849
	f.BoolVar(&lc.json, "json", false, "print full test details as JSON")
	lc.cfg.SetFlags(f)
}

func (lc *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

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
	lc.cfg.Logger = logging.NewSimple(&b, true, true)

	state := config.State{}
	if err := run.SetupGrpcServices(ctx, lc.cfg, &state); err != nil {
		lg.Logf("Failed to set up GRPC servers: %v", err)
		return subcommands.ExitFailure
	}
	if err := run.ResolveHosts(ctx, lc.cfg, &state); err != nil {
		lg.Logf("Failed to resolve hosts: %v", err)
		return subcommands.ExitFailure
	}

	status, results := lc.wrapper.run(ctx, lc.cfg, &state)
	if status.ExitCode != subcommands.ExitSuccess {
		os.Stderr.Write(b.Bytes())
		return status.ExitCode
	}
	tests := make([]*resultsjson.Test, len(results))
	for i := range results {
		tests[i] = &results[i].Test
	}

	if err := lc.printTests(tests); err != nil {
		lg.Log("Failed to write tests: ", err)
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
