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

	"chromiumos/tast/command"
	"chromiumos/tast/crash"
	"chromiumos/tast/logs"
)

const (
	maxCrashesPerExec  = 3                   // max crashes to collect per executable
	journaldCompactLog = "journal.log"       // compact human-readable journald log
	journaldExportLog  = "journal.export.gz" // full compressed journald log with metadata
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

	if cfg.JournaldSubdir != "" {
		if res.State.JournaldCursor, err = logs.GetJournaldCursor(ctx); err != nil {
			// journald isn't widely used yet, so just warn about errors.
			res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to get journald cursor: %v", err))
		}
	}

	if res.State.MinidumpPaths, err = getMinidumps(cfg.SystemCrashDirs); err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(res)
}

// handleCollectSysInfo copies system information that was written after args.CollectSysInfoArgs.InitialState
// was generated into temporary directories and writes a JSON-marshaled CollectSysInfoResult struct to w.
func handleCollectSysInfo(ctx context.Context, args *Args, cfg *Config, w io.Writer) error {
	if cfg.SystemLogDir == "" || len(cfg.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	cmdArgs := &args.CollectSysInfoArgs
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

	// Write journald logs into a subdirectory.
	if subdir := cfg.JournaldSubdir; subdir != "" {
		if cursor := cmdArgs.InitialState.JournaldCursor; cursor != "" {
			if err := writeJournaldLog(ctx, filepath.Join(res.LogDir, subdir, journaldCompactLog), cursor, logs.JournaldCompact); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect compact journald entries: %v", err))
			}
			if err := writeJournaldLog(ctx, filepath.Join(res.LogDir, subdir, journaldExportLog), cursor, logs.JournaldExport); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to collect exported journald entries: %v", err))
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

// writeJournaldLog writes all journald log entries generated after cursor to path using fm,
// creating the parent directory if necessary.
func writeJournaldLog(ctx context.Context, path, cursor string, fm logs.JournaldFormat) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	err = logs.WriteJournaldLogs(ctx, f, cursor, fm)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
