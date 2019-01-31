// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"os"

	"github.com/google/subcommands"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"
)

// downloadCmd implements subcommands.Command to support downloading private test bundles.
type downloadCmd struct {
	cfg     *run.Config // shared config for listing tests
	wrapper runWrapper  // wraps calls to run package
}

// newDownloadCmd returns a new downloadCmd that will write tests to w.
func newDownloadCmd() *downloadCmd {
	return &downloadCmd{
		cfg:     run.NewConfig(run.DownloadBundlesMode, tastDir, trunkDir()),
		wrapper: &realRunWrapper{},
	}
}

func (*downloadCmd) Name() string     { return "download" }
func (*downloadCmd) Synopsis() string { return "download private test bundles" }
func (*downloadCmd) Usage() string {
	return `download <flags> <target>:
	Downloads private test bundles to the DUT.
`
}

func (dc *downloadCmd) SetFlags(f *flag.FlagSet) {
	dc.cfg.SetFlags(f)
}

func (dc *downloadCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + dc.Usage())
		return subcommands.ExitUsageError
	}

	if err := dc.cfg.DeriveDefaults(); err != nil {
		lg.Log("Failed to derive defaults: ", err)
		return subcommands.ExitUsageError
	}

	dc.cfg.Target = f.Args()[0]

	b := bytes.Buffer{}
	dc.cfg.Logger = logging.NewSimple(&b, log.LstdFlags, true)

	status, _ := dc.wrapper.run(ctx, dc.cfg)
	if status.ExitCode != subcommands.ExitSuccess {
		os.Stderr.Write(b.Bytes())
		return status.ExitCode
	}
	return subcommands.ExitSuccess
}
