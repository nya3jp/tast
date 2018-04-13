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

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"
	"chromiumos/tast/testing"

	"github.com/google/subcommands"
)

// listCmd implements subcommands.Command to support listing tests.
type listCmd struct {
	json    bool       // marshal tests to JSON instead of just printing names
	cfg     run.Config // shared config for listing tests
	wrapper runWrapper // wraps calls to run package
	w       io.Writer  // where to write tests
}

// newListCmd returns a new listCmd that will write tests to w.
func newListCmd(w io.Writer) *listCmd {
	return &listCmd{
		cfg:     run.Config{Mode: run.ListTestsMode},
		wrapper: &realRunWrapper{},
		w:       w,
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
	// TODO(derat): Add -listtype: https://crbug.com/831849
	f.BoolVar(&lc.json, "json", false, "print full test details as JSON")

	td := getTrunkDir()
	lc.cfg.SetFlags(f, td)
}

func (lc *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + lc.Usage())
		return subcommands.ExitUsageError
	}

	lc.cfg.Target = f.Args()[0]
	lc.cfg.Patterns = f.Args()[1:]

	b := bytes.Buffer{}
	lc.cfg.Logger = logging.NewSimple(&b, log.LstdFlags, true)

	status, results := lc.wrapper.run(ctx, &lc.cfg)
	if status != subcommands.ExitSuccess {
		os.Stderr.Write(b.Bytes())
		return status
	}
	if err := lc.printTests(results); err != nil {
		lg.Log("Failed to write tests: ", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// printTests writes the tests within results to lc.w.
func (lc *listCmd) printTests(results []run.TestResult) error {
	if lc.json {
		ts := make([]*testing.Test, len(results))
		for i := 0; i < len(ts); i++ {
			ts[i] = &results[i].Test
		}
		enc := json.NewEncoder(lc.w)
		enc.SetIndent("", "  ")
		return enc.Encode(ts)
	}

	// Otherwise, just print test names, one per line.
	for _, res := range results {
		if _, err := fmt.Fprintf(lc.w, "%s\n", res.Test.Name); err != nil {
			return err
		}
	}
	return nil
}
