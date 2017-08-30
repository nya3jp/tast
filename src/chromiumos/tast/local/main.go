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
	"log"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/common/control"
	"chromiumos/tast/common/runner"
	"chromiumos/tast/common/testing"
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

// runTests runs the supplied tests sequentially.
// baseOutDir contains the base directory under which output will be written.
// dataDir contains the base directory under which test data files are located.
// arch contains the machine architecture. The number of failing tests is returned.
func runTests(tests []*testing.Test, mw *control.MessageWriter, baseOutDir, dataDir, arch string) (numFailed int) {
	for _, test := range tests {
		if mw != nil {
			mw.WriteMessage(&control.TestStart{time.Now(), test.Name})
		} else {
			log.Print("Running ", test.Name)
		}

		outDir := filepath.Join(baseOutDir, test.Name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			runner.Abort(mw, err.Error())
		}

		ch := make(chan testing.Output)
		s := testing.NewState(context.Background(), ch, arch, filepath.Join(dataDir, test.DataDir()), outDir, testTimeout)

		done := make(chan bool, 1)
		go func() {
			if succeeded := runner.CopyTestOutput(ch, mw); !succeeded {
				numFailed++
			}
			done <- true
		}()
		test.Run(s)
		close(ch)
		<-done

		if mw != nil {
			mw.WriteMessage(&control.TestEnd{time.Now(), test.Name})
		} else {
			log.Printf("Finished %s", test.Name)
		}
	}

	return numFailed
}

func main() {
	arch := flag.String("arch", "", "machine architecture (see \"uname -m\")")
	dataDir := flag.String("datadir", "", "directory where data files are located")
	listData := flag.Bool("listdata", false, "print data files needed for tests and exit")
	report := flag.Bool("report", false, "report progress for calling process")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
			"Runs local tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	var mw *control.MessageWriter
	if *report {
		mw = control.NewMessageWriter(os.Stdout)
	}

	tests, err := runner.TestsToRun(flag.Args())
	if err != nil {
		runner.Abort(mw, err.Error())
	}

	if *listData {
		if err := listDataFiles(os.Stdout, tests, *arch); err != nil {
			runner.Abort(mw, err.Error())
		}
		os.Exit(0)
	}

	baseOutDir, err := ioutil.TempDir("", "local_tests_data.")
	if err != nil {
		runner.Abort(mw, err.Error())
	}

	var logSizes logs.InodeSizes
	if *report {
		mw.WriteMessage(&control.RunStart{time.Now(), len(tests)})
		logSizes = getInitialLogSizes(mw)
	}

	numFailed := runTests(tests, mw, baseOutDir, *dataDir, *arch)

	if *report {
		ld := copyLogUpdates(logSizes, mw)
		mw.WriteMessage(&control.RunEnd{time.Now(), ld, baseOutDir})
	}

	// Exit with a nonzero exit code if we were run manually and saw at least one test fail.
	if !*report && numFailed > 0 {
		os.Exit(1)
	}
}
