// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"chromiumos/cmd/tast/symbolize"

	"github.com/google/subcommands"
)

// symbolizeCmd implements subcommands.Command to support symbolizing crashes.
type symbolizeCmd struct {
	cfg symbolize.Config
}

func (*symbolizeCmd) Name() string     { return "symbolize" }
func (*symbolizeCmd) Synopsis() string { return "symbolize crashes" }
func (*symbolizeCmd) Usage() string {
	return `symbolize <flags> <file>:
	Symbolizes a crash dump.
`
}

func (s *symbolizeCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.cfg.SymbolDir, "symboldir", "/tmp/breakpad_symbols", "directory to write symbol files to")
	f.StringVar(&s.cfg.BuildRoot, "buildroot", "", "buildroot containing debugging binaries; inferred from dump if empty")
}

func (s *symbolizeCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(f.Args()) != 1 {
		fmt.Fprintf(os.Stderr, s.Usage())
		return subcommands.ExitUsageError
	}

	s.cfg.Logger = lg
	path := f.Args()[0]
	if err := symbolize.SymbolizeCrash(path, os.Stdout, s.cfg); err != nil {
		lg.Logf("Failed to symbolize %v: %v", path, err)
	}
	return subcommands.ExitSuccess
}
