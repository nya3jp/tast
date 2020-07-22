// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	gotesting "testing"

	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/runner"
)

func TestRunTestsFailureBeforeRun(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Make the runner always fail, and ask to check test deps so we'll get a failure before trying
	// to run tests. local() shouldn't set startedRun to true since we failed before then.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) { return 1 }
	td.cfg.checkTestDeps = true
	if _, err := runTests(context.Background(), &td.cfg); err == nil {
		t.Errorf("runTests unexpectedly passed")
	} else if td.cfg.startedRun {
		t.Error("runTests incorrectly reported that run was started after early failure")
	}
}

func TestRunTestsGetDUTInfo(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

	chromeOSVersion := "13312.0.2020_07_02_1108"

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.GetDUTInfoMode:
			// Just check that getDUTInfo is called; details of args are
			// tested in deps_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&runner.GetDUTInfoResult{
				SoftwareFeatures: &dep.SoftwareFeatures{
					Available: []string{"foo"}, // must report non-empty features
				},
				ChromeOSVersion: chromeOSVersion,
			})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.checkTestDeps = true

	if _, err := runTests(context.Background(), &td.cfg); err != nil {
		t.Error("runTests failed: ", err)
	}
	/* Temporary comment out until we find a way to check full.txt
	// check if the Chrome OS Version is in the log file
	fullLogName := filepath.Join(td.cfg.ResDir, "full.txt")

	foundLine, err := grepFirstLine(fullLogName, "Chrome OS Version: "+chromeOSVersion)
	if err != nil {
		t.Error("Fail to grep from "+fullLogName, err)
	}

	if foundLine == "" {
		t.Errorf("Cannot find Chrome OS Version in %v", fullLogName)
	}
	*/
	if !called {
		t.Error("runTests did not call getSoftwareFeatures")
	}
}

func TestRunTestsGetInitialSysInfo(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.GetSysInfoStateMode:
			// Just check that getInitialSysInfo is called; details of args are
			// tested in sys_info_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&runner.GetSysInfoStateResult{})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.collectSysInfo = true

	if _, err := runTests(context.Background(), &td.cfg); err != nil {
		t.Error("runTests failed: ", err)
	}
	if !called {
		t.Errorf("runTests did not call getInitialSysInfo")
	}
}

// grepFirstLine greps for the first line contains the string in argument
func grepFirstLine(fileName, searchStr string) (string, error) {

	fileHandle, err := os.Open(fileName)
	if err != nil {
		return "", err
	}
	defer fileHandle.Close()

	sc := bufio.NewScanner(fileHandle)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, searchStr) {
			return line, nil
		}
	}
	return "", nil
}
