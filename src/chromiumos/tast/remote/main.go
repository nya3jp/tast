// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements an executable containing remote tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/common/control"
	"chromiumos/tast/common/runner"
	"chromiumos/tast/remote/dut"

	// These packages register their tests via init() functions.
	_ "chromiumos/tast/remote/tests/power"
)

const (
	dutTimeout  = 10 * time.Second
	testTimeout = 5 * time.Minute
)

func main() {
	cfg := runner.RunConfig{TestTimeout: testTimeout}

	flag.StringVar(&cfg.Arch, "arch", "", "target machine architecture (see \"uname -m\")")
	flag.StringVar(&cfg.DataDir, "datadir", "", "directory where data files are located")
	target := flag.String("target", "", "DUT connection spec as \"[<user>@]host[:<port>]\"")
	keypath := flag.String("keypath", "", "path to SSH private key to use for connecting to DUT")
	report := flag.Bool("report", false, "report progress for calling process")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
			"Runs remote tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if *report {
		cfg.MessageWriter = control.NewMessageWriter(os.Stdout)
	}

	var err error
	if cfg.Tests, err = runner.TestsToRun(flag.Args()); err != nil {
		runner.Abort(cfg.MessageWriter, err.Error())
	}

	dt, err := dut.New(*target, *keypath)
	if err = dt.Reconnect(context.Background()); err != nil {
		runner.Abort(cfg.MessageWriter, fmt.Sprintf("failed to connect to DUT: %v", err))
	}
	defer dt.Close(context.Background())

	cfg.Ctx = dut.NewContext(context.Background(), dt)
	cfg.SetupFunc = func() error {
		if !dt.Connected(cfg.Ctx) {
			return dt.Reconnect(cfg.Ctx)
		}
		return nil
	}

	if cfg.BaseOutDir, err = ioutil.TempDir("", "remote_tests_data."); err != nil {
		runner.Abort(cfg.MessageWriter, err.Error())
	}

	// Perform the test run.
	if *report {
		cfg.MessageWriter.WriteMessage(&control.RunStart{time.Now(), len(cfg.Tests)})
	}
	numFailed, err := runner.RunTests(cfg)
	if err != nil {
		runner.Abort(cfg.MessageWriter, err.Error())
	}
	if *report {
		cfg.MessageWriter.WriteMessage(&control.RunEnd{time.Now(), "", cfg.BaseOutDir})
	}

	// Exit with a nonzero exit code if we were run manually and saw at least one test fail.
	if !*report && numFailed > 0 {
		os.Exit(1)
	}
}
