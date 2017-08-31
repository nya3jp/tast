// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements an executable containing local tests.
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
	"chromiumos/tast/local/logs"

	// These packages register their tests via init functions.
	_ "chromiumos/tast/local/tests/example"
	_ "chromiumos/tast/local/tests/power"
	_ "chromiumos/tast/local/tests/security"
	_ "chromiumos/tast/local/tests/ui"
)

const (
	systemLogDir = "/var/log"  // directory where system logs are located
	testTimeout  = time.Minute // maximum running time for a test
)

// getInitialLogSizes returns the starting sizes of log files.
// If mw is nil, messages are logged to stdout instead.
func getInitialLogSizes(mw *control.MessageWriter) logs.InodeSizes {
	runner.Log(mw, "Getting original log inode sizes")
	ls, warnings, err := logs.GetLogInodeSizes(systemLogDir)
	for p, err := range warnings {
		runner.Log(mw, fmt.Sprintf("Failed to check log %s: %v", p, err))
	}
	if err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to get original log inode sizes: %v", err))
	}
	return ls
}

// copyLogUpdates copies updated portions of system logs to a temporary dir.
// sizes contains the original log sizes and is used to identify old content that won't be copied.
// If mw is nil, messages are logged to stdout instead.
// The directory containing the log updates is returned.
func copyLogUpdates(sizes logs.InodeSizes, mw *control.MessageWriter) (outDir string) {
	runner.Log(mw, "Copying log updates")
	if sizes == nil {
		runner.Log(mw, "Don't have original log sizes")
		return
	}

	var err error
	if outDir, err = ioutil.TempDir("", "local_tests_logs."); err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to create log output dir: %v", err))
		return
	}

	warnings, err := logs.CopyLogFileUpdates(systemLogDir, outDir, sizes)
	for p, werr := range warnings {
		runner.Log(mw, fmt.Sprintf("Failed to copy log %s: %v", p, werr))
	}
	if err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to copy log updates: %v", err))
	}
	return outDir
}

func main() {
	cfg := runner.RunConfig{
		Ctx:         context.Background(),
		TestTimeout: testTimeout,
	}

	flag.StringVar(&cfg.Arch, "arch", "", "machine architecture (see \"uname -m\")")
	flag.StringVar(&cfg.DataDir, "datadir", "", "directory where data files are located")
	listData := flag.Bool("listdata", false, "print data files needed for tests and exit")
	report := flag.Bool("report", false, "report progress for calling process")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
			"Runs local tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
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

	if *listData {
		if err := listDataFiles(os.Stdout, cfg.Tests, cfg.Arch); err != nil {
			runner.Abort(cfg.MessageWriter, err.Error())
		}
		os.Exit(0)
	}

	if cfg.BaseOutDir, err = ioutil.TempDir("", "local_tests_data."); err != nil {
		runner.Abort(cfg.MessageWriter, err.Error())
	}

	// Perform the test run.
	var logSizes logs.InodeSizes
	if *report {
		cfg.MessageWriter.WriteMessage(&control.RunStart{time.Now(), len(cfg.Tests)})
		logSizes = getInitialLogSizes(cfg.MessageWriter)
	}
	numFailed, err := runner.RunTests(cfg)
	if err != nil {
		runner.Abort(cfg.MessageWriter, err.Error())
	}
	if *report {
		logDir := copyLogUpdates(logSizes, cfg.MessageWriter)
		cfg.MessageWriter.WriteMessage(&control.RunEnd{time.Now(), logDir, cfg.BaseOutDir})
	}

	// Exit with a nonzero exit code if we were run manually and saw at least one test fail.
	if !*report && numFailed > 0 {
		os.Exit(1)
	}
}
