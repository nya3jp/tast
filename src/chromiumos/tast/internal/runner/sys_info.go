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
	maxCrashesPerExec         = 3                   // max crashes to collect per executable
	unifiedCompactLogFileName = "unified.log"       // compact human-readable unified system log
	unifiedExportLogFileName  = "unified.export.gz" // full compressed unified system log with metadata
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

	if cfg.UnifiedLogSubdir != "" {
		if res.State.UnifiedLogCursor, err = logs.GetUnifiedLogCursor(ctx); err != nil {
			// croslog may not be installed, so just warn about errors.
			res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to get unified system log cursor: %v", err))
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
	if subdir := cfg.UnifiedLogSubdir; subdir != "" {
		if cursor := cmdArgs.InitialState.UnifiedLogCursor; cursor != "" {
			if err := writeUnifiedLog(ctx, filepath.Join(res.LogDir, subdir, unifiedCompactLogFileName), cursor, logs.CompactLogFormat); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect compact unified system log entries: %v", err))
			}
			if err := writeUnifiedLog(ctx, filepath.Join(res.LogDir, subdir, unifiedExportLogFileName), cursor, logs.GzippedExportLogFormat); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect exported unified system log entries: %v", err))
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

// writeUnifiedLog writes unified system log entries generated after cursor to path using fm,
// creating the parent directory if necessary.
func writeUnifiedLog(ctx context.Context, path, cursor string, fm logs.UnifiedLogFormat) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	err = logs.ExportUnifiedLogs(ctx, f, cursor, fm)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
