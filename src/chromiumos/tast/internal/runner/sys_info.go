// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/crash"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logs"
)

const (
	unifiedCompactLogFileName = "unified.log"       // compact human-readable unified system log
	unifiedExportLogFileName  = "unified.export.gz" // full compressed unified system log with metadata
)

// handleGetSysInfoState gets information about the system's current state (e.g. log files
// and crash reports) and writes a JSON-marshaled RunnerGetSysInfoStateResult struct to w.
func handleGetSysInfoState(ctx context.Context, scfg *StaticConfig, w io.Writer) error {
	logger := newArrayLogger()
	ctx = logging.AttachLogger(ctx, logger)

	if scfg.SystemLogDir == "" || len(scfg.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	if err := suspendLogCleanup(scfg.CleanupLogsPausedPath); err != nil {
		logging.Infof(ctx, "Failed to pause log cleanup: %v", err)
	}

	logInodeSizes, err := logs.GetLogInodeSizes(ctx, scfg.SystemLogDir, scfg.SystemLogExcludes)
	if err != nil {
		return err
	}

	var unifiedLogCursor string
	if scfg.UnifiedLogSubdir != "" {
		unifiedLogCursor, err = logs.GetUnifiedLogCursor(ctx)
		if err != nil {
			// croslog may not be installed, so just warn about errors.
			logging.Infof(ctx, "Failed to get unified system log cursor: %v", err)
		}
	}

	minidumpPaths, err := getMinidumps(scfg.SystemCrashDirs)
	if err != nil {
		return err
	}

	res := &jsonprotocol.RunnerGetSysInfoStateResult{
		State: jsonprotocol.SysInfoState{
			LogInodeSizes:    logInodeSizes,
			UnifiedLogCursor: unifiedLogCursor,
			MinidumpPaths:    minidumpPaths,
		},
		Warnings: logger.Logs(),
	}
	return json.NewEncoder(w).Encode(res)
}

// handleCollectSysInfo copies system information that was written after args.CollectSysInfo.InitialState
// was generated into temporary directories and writes a JSON-marshaled RunnerCollectSysInfoResult struct to w.
func handleCollectSysInfo(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, w io.Writer) error {
	logger := newArrayLogger()
	ctx = logging.AttachLogger(ctx, logger)

	if scfg.SystemLogDir == "" || len(scfg.SystemCrashDirs) == 0 {
		return command.NewStatusErrorf(statusBadArgs, "system info collection unsupported")
	}

	cmdArgs := args.CollectSysInfo
	if cmdArgs == nil {
		return command.NewStatusErrorf(statusBadArgs, "missing system info args")
	}

	// Collect syslog logs.
	logDir, err := ioutil.TempDir("", "tast_logs.")
	if err != nil {
		return err
	}
	if err := logs.CopyLogFileUpdates(ctx, scfg.SystemLogDir, logDir, scfg.SystemLogExcludes,
		cmdArgs.InitialState.LogInodeSizes); err != nil {
		return err
	}

	// Write system logs into a subdirectory.
	if subdir := scfg.UnifiedLogSubdir; subdir != "" {
		if cursor := cmdArgs.InitialState.UnifiedLogCursor; cursor != "" {
			if err := writeUnifiedLog(ctx, filepath.Join(logDir, subdir, unifiedCompactLogFileName), cursor, logs.CompactLogFormat); err != nil {
				logging.Infof(ctx, "Failed to collect compact unified system log entries: %v", err)
			}
			if err := writeUnifiedLog(ctx, filepath.Join(logDir, subdir, unifiedExportLogFileName), cursor, logs.GzippedExportLogFormat); err != nil {
				logging.Infof(ctx, "Failed to collect exported unified system log entries: %v", err)
			}
		}
	}

	// Collect additional information.
	if scfg.SystemInfoFunc != nil {
		if err := scfg.SystemInfoFunc(ctx, logDir); err != nil {
			logging.Infof(ctx, "Failed to collect additional system info: %v", err)
		}
	}

	// Collect crashes.
	dumps, err := getMinidumps(scfg.SystemCrashDirs)
	if err != nil {
		return err
	}
	crashDir, err := ioutil.TempDir("", "tast_crashes.")
	if err != nil {
		return err
	}
	if err := crash.CopyNewFiles(ctx, crashDir, dumps, cmdArgs.InitialState.MinidumpPaths); err != nil {
		return err
	}
	// TODO(derat): Decide if it's worthwhile to call crash.CopySystemInfo here to get /etc/lsb-release.
	// Doing so makes it harder to exercise this code in unit tests.

	if err := resumeLogCleanup(scfg.CleanupLogsPausedPath); err != nil {
		logging.Infof(ctx, "Failed to resume log cleanup: %v", err)
	}

	res := &jsonprotocol.RunnerCollectSysInfoResult{
		LogDir:   logDir,
		CrashDir: crashDir,
		Warnings: logger.Logs(),
	}
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

func suspendLogCleanup(cleanupLogsPausedPath string) error {
	return ioutil.WriteFile(cleanupLogsPausedPath, nil, 0666)
}

func resumeLogCleanup(cleanupLogsPausedPath string) error {
	return os.Remove(cleanupLogsPausedPath)
}
