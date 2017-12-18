// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the local_test_runner executable.
//
// local_test_runner is executed on-device by the tast command.
// It runs test bundles and reports the results back to tast.
package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"chromiumos/tast/control"
	"chromiumos/tast/crash"
	"chromiumos/tast/logs"
	"chromiumos/tast/runner"
)

const (
	defaultBundleGlob = "/usr/local/libexec/tast/bundles/*" // default glob matching test bundles
	defaultDataDir    = "/usr/local/share/tast/data"        // default dir containing test data

	systemLogDir      = "/var/log" // directory where system logs are located
	maxCrashesPerExec = 3          // max crashes to collect per executable
)

// getInitialLogSizes returns the starting sizes of log files.
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

// getMinidumps returns paths of all minidump files on the system.
func getMinidumps() ([]string, error) {
	all := make([]string, 0)
	for _, dir := range []string{crash.DefaultCrashDir, crash.ChromeCrashDir} {
		if _, ds, err := crash.GetCrashes(dir); err != nil {
			return nil, err
		} else {
			all = append(all, ds...)
		}
	}
	return all, nil
}

// copyNewMinidumps copies new minidump crash reports into a temporary dir.
// oldDumps contains paths of dump files that existed before the test run started.
func copyNewMinidumps(oldDumps []string, mw *control.MessageWriter) (outDir string) {
	runner.Log(mw, "Copying crashes")
	newDumps, err := getMinidumps()
	if err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to get new crashes: %v", err))
		return
	}
	if outDir, err = ioutil.TempDir("", "local_tests_crashes."); err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to create minidump output dir: %v", err))
		return
	}

	warnings, err := crash.CopyNewFiles(outDir, newDumps, oldDumps, maxCrashesPerExec)
	for p, werr := range warnings {
		runner.Log(mw, fmt.Sprintf("Failed to copy minidump %s: %v", p, werr))
	}
	if err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to copy minidumps: %v", err))
	}
	if err = crash.CopySystemInfo(outDir); err != nil {
		runner.Log(mw, fmt.Sprintf("Failed to copy crash-related system info: %v", err))
	}
	return outDir
}

func main() {
	cfg, status := runner.ParseArgs(os.Stdout, os.Args[1:], defaultBundleGlob, defaultDataDir, nil)
	if status != 0 || cfg == nil {
		os.Exit(status)
	}

	var logSizes logs.InodeSizes
	var oldMinidumps []string
	cfg.PreRun = func(mw *control.MessageWriter) {
		logSizes = getInitialLogSizes(mw)
		var err error
		if oldMinidumps, err = getMinidumps(); err != nil {
			runner.Log(mw, fmt.Sprintf("Failed to get existing minidumps: %v", err))
		}
	}
	cfg.PostRun = func(mw *control.MessageWriter) control.RunEnd {
		logDir := copyLogUpdates(logSizes, mw)
		crashDir := copyNewMinidumps(oldMinidumps, mw)
		return control.RunEnd{LogDir: logDir, CrashDir: crashDir}
	}

	os.Exit(runner.RunTests(cfg))
}
