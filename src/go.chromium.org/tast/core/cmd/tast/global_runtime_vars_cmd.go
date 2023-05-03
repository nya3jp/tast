// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/google/subcommands"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/internal/logging"
)

// symbolizeCmd implements subcommands.Command to support symbolizing crashes.
type globalRuntimeVarsCmd struct {
	cfg     *config.MutableConfig       // shared config for listing tests
	wrapper globalRuntimeVarsrunWrapper // wraps calls to run package
	stdout  io.Writer                   // where to write tests
}

var _ = subcommands.Command(&globalRuntimeVarsCmd{})

// newGlobalRuntimeVarsCmd returns a new globalRuntimeVarsCmd that will write tests to stdout.
func newGlobalRuntimeVarsCmd(stdout io.Writer, trunkDir string) *globalRuntimeVarsCmd {
	return &globalRuntimeVarsCmd{
		cfg:     config.NewMutableConfig(config.GlobalRuntimeVarsMode, tastDir, trunkDir),
		wrapper: &realRunWrapper{},
		stdout:  stdout,
	}
}

func (*globalRuntimeVarsCmd) Name() string { return "globalruntimevars" }
func (*globalRuntimeVarsCmd) Synopsis() string {
	return "list all runtime variables currently defined in tast"
}
func (*globalRuntimeVarsCmd) Usage() string {
	return `Usage: globalruntimevars [flag]... <target>

Description:
	List all currently registered global runtime variables.

Target:
    The target is an SSH connection spec of the form "[user@]host[:port]".

Flag:
`
}

func (gc *globalRuntimeVarsCmd) SetFlags(f *flag.FlagSet) {
	gc.cfg.SetFlags(f)
}

func (gc *globalRuntimeVarsCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(f.Args()) == 0 {
		logging.Info(ctx, "Missing target.\n\n"+gc.Usage())
		return subcommands.ExitUsageError
	}
	if err := gc.cfg.DeriveDefaults(); err != nil {
		logging.Info(ctx, "Failed to derive defaults: ", err)
		return subcommands.ExitUsageError
	}
	gc.cfg.Target = f.Args()[0]

	result, err := gc.wrapper.GlobalRuntimeVars(ctx, gc.cfg.Freeze(), &config.DeprecatedState{})

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return subcommands.ExitFailure
	}

	for _, t := range result {
		if _, err := fmt.Fprintln(gc.stdout, t); err != nil {
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				return subcommands.ExitFailure
			}
		}

	}
	return subcommands.ExitSuccess
}
