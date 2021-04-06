// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/subcommands"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/symbolize"
)

// symbolizeCmd implements subcommands.Command to support symbolizing crashes.
type symbolizeCmd struct {
	cfg symbolize.Config
}

var _ = subcommands.Command(&runCmd{})

func (*symbolizeCmd) Name() string     { return "symbolize" }
func (*symbolizeCmd) Synopsis() string { return "symbolize crashes" }
func (*symbolizeCmd) Usage() string {
	return `Usage: symbolize [flag]... <file>

Symbolize a minidump crash file to stdout.

`
}

func (s *symbolizeCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.cfg.SymbolDir, "symboldir", "/tmp/breakpad_symbols", "directory to write symbol files to")
	f.StringVar(&s.cfg.BuilderPath, "builderpath", "",
		"for example, betty-release/R91-13892.0.0, it can be found in /etc/lsb-release; inferred from dump if empty")
	f.StringVar(&s.cfg.BuildRoot, "buildroot", "",
		"buildroot containing debugging binaries, for example /build/betty; inferred from dump if empty")
}

func (s *symbolizeCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

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
