// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"chromiumos/tast/command"
	"chromiumos/tast/crash"
	"chromiumos/tast/logs"
)

const (
	maxCrashesPerExec = 3 // max crashes to collect per executable
)

// handleGetSysInfoState gets information about the system's current state (e.g. log files
// and crash reports) and writes a JSON-marshaled GetSysInfoStateResult struct to w.
func handleGetSysInfoState(args *Args, w io.Writer) error {
	if args.SystemLogDir == "" || len(args.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	res := GetSysInfoStateResult{}

	var err error
	var warnings map[string]error
	res.State.LogInodeSizes, warnings, err = logs.GetLogInodeSizes(args.SystemLogDir)
	if err != nil {
		return err
	}
	for p, warning := range warnings {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", p, warning))
	}

	if res.State.MinidumpPaths, err = getMinidumps(args.SystemCrashDirs); err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(res)
}

// handleCollectSysInfo copies system information that was written after args.CollectSysInfoArgs.InitialState
// was generated into temporary directories and writes a JSON-marshaled CollectSysInfoResult struct to w.
func handleCollectSysInfo(args *Args, w io.Writer) error {
	if args.SystemLogDir == "" || len(args.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	cmdArgs := &args.CollectSysInfoArgs
	res := CollectSysInfoResult{}

	// Collect logs.
	var err error
	if res.LogDir, err = ioutil.TempDir("", "tast_logs."); err != nil {
		return err
	}
	var warnings map[string]error
	if warnings, err = logs.CopyLogFileUpdates(args.SystemLogDir, res.LogDir, cmdArgs.InitialState.LogInodeSizes); err != nil {
		return err
	}
	for p, w := range warnings {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", p, w))
	}

	// Collect crashes.
	dumps, err := getMinidumps(args.SystemCrashDirs)
	if err != nil {
		return err
	}
	if res.CrashDir, err = ioutil.TempDir("", "tast_crashes."); err != nil {
		return err
	}
	if warnings, err = crash.CopyNewFiles(res.CrashDir, dumps, cmdArgs.InitialState.MinidumpPaths, maxCrashesPerExec); err != nil {
		return err
	}
	for p, w := range warnings {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", p, w))
	}
	// TODO(derat): Decide if it's worthwhile to call crash.CopySystemInfo here to get /etc/lsb-release.
	// Doing so makes it harder to exercise this code in unit tests.

	return json.NewEncoder(w).Encode(res)
}

// getMinidumps returns the paths of all minidump files within dirs.
func getMinidumps(dirs []string) ([]string, error) {
	all := make([]string, 0)
	for _, dir := range dirs {
		if _, ds, err := crash.GetCrashes(dir); err != nil {
			return nil, err
		} else {
			all = append(all, ds...)
		}
	}
	return all, nil
}
