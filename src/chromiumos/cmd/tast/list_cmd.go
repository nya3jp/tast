// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"

	"github.com/google/subcommands"
)

// listCmd implements subcommands.Command to support listing tests.
type listCmd struct {
	testType string     // type of tests to list (either "local" or "remote")
	json     bool       // marshal tests to JSON instead of just printing names
	cfg      run.Config // shared config for listing tests
}

func newListCmd() *listCmd {
	return &listCmd{
		cfg: run.Config{
			PrintMode: run.PrintNames,
			PrintDest: os.Stdout,
		},
	}
}

func (*listCmd) Name() string     { return "list" }
func (*listCmd) Synopsis() string { return "list tests" }
func (*listCmd) Usage() string {
	return `list <flags> <target> <pattern> <pattern> ...:
	Lists tests matched by one or more patterns.
`
}

func (lc *listCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&lc.testType, "type", "local", "type of tests to list (either \"local\" or \"remote\")")
	f.BoolVar(&lc.json, "json", false, "print full test details as JSON")

	td := getTrunkDir()
	lc.cfg.SetFlags(f, td)
	lc.cfg.BuildCfg.SetFlags(f, td)
}

func (lc *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + lc.Usage())
		return subcommands.ExitUsageError
	}

	lc.cfg.Target = f.Args()[0]
	lc.cfg.Patterns = f.Args()[1:]

	if lc.json {
		lc.cfg.PrintMode = run.PrintJSON
	}

	b := bytes.Buffer{}
	lc.cfg.Logger = logging.NewSimple(&b, log.LstdFlags, true)

	var status subcommands.ExitStatus
	switch lc.testType {
	case localType:
		status, _ = run.Local(ctx, &lc.cfg)
	case remoteType:
		status, _ = run.Remote(ctx, &lc.cfg)
	default:
		lg.Logf(fmt.Sprintf("Invalid test type %q\n\n%s", lc.testType, lc.Usage()))
		return subcommands.ExitUsageError
	}
	if status != subcommands.ExitSuccess {
		os.Stderr.Write(b.Bytes())
	}
	return status
}
