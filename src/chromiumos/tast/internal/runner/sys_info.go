// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/crash"
	"chromiumos/tast/internal/logs"
)

const (
	maxCrashesPerExec  = 3                    // max crashes to collect per executable
	compactLogFileName = "combined.log"       // compact human-readable system log
	exportLogFileName  = "combined.export.gz" // full compressed system log with metadata
)

// handleGetSysInfoState gets information about the system's current state (e.g. log files
// and crash reports) and writes a JSON-marshaled GetSysInfoStateResult struct to w.
func handleGetSysInfoState(ctx context.Context, cfg *Config, w io.Writer) error {
	if cfg.SystemLogDir == "" || len(cfg.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	res := GetSysInfoStateResult{}

	var err error
	var warnings map[string]error
	if res.State.LogInodeSizes, warnings, err = logs.GetLogInodeSizes(
		cfg.SystemLogDir, cfg.SystemLogExcludes); err != nil {
		return err
	}
	for p, warning := range warnings {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", p, warning))
	}

	if cfg.CombinedLogSubdir != "" {
		if res.State.SystemLogCursor, err = logs.GetSystemLogCursor(ctx); err != nil {
			// croslog may not installed, so just warn about errors.
			res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to get system log cursor: %v", err))
		}
	}

	if res.State.MinidumpPaths, err = getMinidumps(cfg.SystemCrashDirs); err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(res)
}

// handleCollectSysInfo copies system information that was written after args.CollectSysInfo.InitialState
// was generated into temporary directories and writes a JSON-marshaled CollectSysInfoResult struct to w.
func handleCollectSysInfo(ctx context.Context, args *Args, cfg *Config, w io.Writer) error {
	if cfg.SystemLogDir == "" || len(cfg.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	cmdArgs := args.CollectSysInfo
	if cmdArgs == nil {
		return command.NewStatusErrorf(statusBadArgs, "missing system info args")
	}
	res := CollectSysInfoResult{}

	// Collect syslog logs.
	var err error
	if res.LogDir, err = ioutil.TempDir("", "tast_logs."); err != nil {
		return err
	}
	var warnings map[string]error
	if warnings, err = logs.CopyLogFileUpdates(cfg.SystemLogDir, res.LogDir, cfg.SystemLogExcludes,
		cmdArgs.InitialState.LogInodeSizes); err != nil {
		return err
	}
	for p, w := range warnings {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", p, w))
	}

	// Write system logs into a subdirectory.
	if subdir := cfg.CombinedLogSubdir; subdir != "" {
		if cursor := cmdArgs.InitialState.SystemLogCursor; cursor != "" {
			if err := writeSystemLog(ctx, filepath.Join(res.LogDir, subdir, compactLogFileName), cursor, logs.CompactLogFormat); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect compact system log entries: %v", err))
			}
			if err := writeSystemLog(ctx, filepath.Join(res.LogDir, subdir, exportLogFileName), cursor, logs.GzippedExportLogFormat); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect exported system log entries: %v", err))
			}
		}
	}

	// Collect additional information.
	if cfg.SystemInfoFunc != nil {
		if err := cfg.SystemInfoFunc(ctx, res.LogDir); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect additional system info: %v", err))
		}
	}

	// Collect crashes.
	dumps, err := getMinidumps(cfg.SystemCrashDirs)
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
	var dumps []string
	paths, err := crash.GetCrashes(dirs...)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if filepath.Ext(path) == crash.MinidumpExt {
			dumps = append(dumps, path)
		}
	}
	return dumps, nil
}

// writeSystemLog writes all system log entries generated after cursor to path using fm,
// creating the parent directory if necessary.
func writeSystemLog(ctx context.Context, path, cursor string, fm logs.SystemLogFormat) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	err = logs.ExportSystemLogs(ctx, f, cursor, fm)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
