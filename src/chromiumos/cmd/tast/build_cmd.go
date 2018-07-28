// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/logging"

	"github.com/google/subcommands"
)

// buildCmd implements subcommands.Command to support compiling executables.
type buildCmd struct {
	cfg      build.Config
	pkg, out string
}

func (*buildCmd) Name() string     { return "build" }
func (*buildCmd) Synopsis() string { return "build tests" }
func (*buildCmd) Usage() string {
	return `build <flags> <pkg> <outdir>:
	Builds an executable package.
`
}

func (b *buildCmd) SetFlags(f *flag.FlagSet) {
	b.cfg.SetFlags(f, getTrunkDir())
}

func (b *buildCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

	if len(f.Args()) != 2 {
		fmt.Fprintf(os.Stderr, b.Usage())
		return subcommands.ExitUsageError
	}

	if b.cfg.Arch == "" {
		var err error
		if b.cfg.Arch, err = build.GetLocalArch(); err != nil {
			lg.Log("Failed to get local arch: ", err)
			return subcommands.ExitFailure
		}
	}

	if out, err := build.Build(ctx, &b.cfg, f.Args()[0], f.Args()[1], ""); err != nil {
		lg.Logf("Failed building: %v\n%s", err, string(out))
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
