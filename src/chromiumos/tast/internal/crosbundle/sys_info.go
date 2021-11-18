// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/crash"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logs"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/shutil"
)

const (
	systemLogDir = "/var/log"

	cleanupLogsPausedPath = "/var/lib/cleanup_logs_paused"

	unifiedLogSubdir          = "unified"           // destination for exported unified system logs
	unifiedCompactLogFileName = "unified.log"       // compact human-readable unified system log
	unifiedExportLogFileName  = "unified.export.gz" // full compressed unified system log with metadata
)

var (
	systemLogExcludes = []string{"journal"} // journald binary logs: https://crbug.com/931951
)

// GetSysInfoState implements the GetSysInfoState RPC method for Chrome OS.
func GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
	if err := suspendLogCleanup(); err != nil {
		logging.Infof(ctx, "Failed to pause log cleanup: %v", err)
	}

	logInodeSizes, err := logs.GetLogInodeSizes(ctx, systemLogDir, systemLogExcludes)
	if err != nil {
		return nil, err
	}

	unifiedLogCursor, err := logs.GetUnifiedLogCursor(ctx)
	if err != nil {
		// croslog may not be installed, so just warn about errors.
		logging.Infof(ctx, "Failed to get unified system log cursor: %v", err)
	}

	minidumpPaths, err := getMinidumps()
	if err != nil {
		return nil, err
	}

	return &protocol.GetSysInfoStateResponse{
		State: &protocol.SysInfoState{
			LogInodeSizes:    logInodeSizes,
			UnifiedLogCursor: unifiedLogCursor,
			MinidumpPaths:    minidumpPaths,
		},
	}, nil
}

// CollectSysInfo implements the CollectSysInfo RPC method for Chrome OS.
func CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
	initialState := req.GetInitialState()

	// Collect syslog logs.
	logDir, err := ioutil.TempDir("", "tast_logs.")
	if err != nil {
		return nil, err
	}
	if err := logs.CopyLogFileUpdates(ctx, systemLogDir, logDir, systemLogExcludes, initialState.GetLogInodeSizes()); err != nil {
		return nil, err
	}

	// Write system logs into a subdirectory.
	if cursor := initialState.GetUnifiedLogCursor(); cursor != "" {
		if err := writeUnifiedLog(ctx, filepath.Join(logDir, unifiedLogSubdir, unifiedCompactLogFileName), cursor, logs.CompactLogFormat); err != nil {
			logging.Infof(ctx, "Failed to collect compact unified system log entries: %v", err)
		}
		if err := writeUnifiedLog(ctx, filepath.Join(logDir, unifiedLogSubdir, unifiedExportLogFileName), cursor, logs.GzippedExportLogFormat); err != nil {
			logging.Infof(ctx, "Failed to collect exported unified system log entries: %v", err)
		}
	}

	// Collect additional information.
	if err := writeSystemInfo(ctx, logDir); err != nil {
		logging.Infof(ctx, "Failed to collect additional system info: %v", err)
	}

	// Collect crashes.
	dumps, err := getMinidumps()
	if err != nil {
		return nil, err
	}
	crashDir, err := ioutil.TempDir("", "tast_crashes.")
	if err != nil {
		return nil, err
	}
	if err := crash.CopyNewFiles(ctx, crashDir, dumps, initialState.GetMinidumpPaths()); err != nil {
		return nil, err
	}
	// TODO(derat): Decide if it's worthwhile to call crash.CopySystemInfo here to get /etc/lsb-release.
	// Doing so makes it harder to exercise this code in unit tests.

	if err := resumeLogCleanup(); err != nil {
		logging.Infof(ctx, "Failed to resume log cleanup: %v", err)
	}

	return &protocol.CollectSysInfoResponse{
		LogDir:   logDir,
		CrashDir: crashDir,
	}, nil
}

// getMinidumps returns the paths of all minidump files within dirs.
func getMinidumps() ([]string, error) {
	var dumps []string
	paths, err := crash.GetCrashes(crash.DefaultDirs()...)
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

// writeSystemInfo writes additional system information from the DUT to files within dir.
func writeSystemInfo(ctx context.Context, dir string) error {
	runCmd := func(cmd *exec.Cmd, fn string) error {
		f, err := os.Create(filepath.Join(dir, fn))
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := fmt.Fprintf(f, "%q at end of testing:\n\n", shutil.EscapeSlice(cmd.Args)); err != nil {
			return err
		}
		cmd.Stdout = f
		cmd.Stderr = f
		return cmd.Run()
	}

	var errs []string
	cmds := map[string][]string{
		"upstart_jobs.txt": {"initctl", "list"},
		"ps.txt":           {"ps", "auxwwf"},
		"du_stateful.txt":  {"du", "-m", "/mnt/stateful_partition"},
		"mount.txt":        {"mount"},
		"hostname.txt":     {"hostname"},
		"uptime.txt":       {"uptime"},
		"losetup.txt":      {"losetup"},
		"lscpu.txt":        {"lscpu"},
		"df.txt":           {"df", "-mP"},
		"dmesg.txt":        {"dmesg"},
	}
	if _, err := os.Stat("/proc/bus/pci"); !os.IsNotExist(err) {
		cmds["lspci.txt"] = []string{"lspci", "-vvnn"}
	}

	for fn, cmd := range cmds {
		// Set timeout in case some commands take long time unexpectedly. (crbug.com/1147723)
		cmdCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
		cmd := exec.CommandContext(cmdCtx, cmd[0], cmd[1:len(cmd)]...)
		if err := runCmd(cmd, fn); err != nil {
			errs = append(errs, fmt.Sprintf("failed running %q: %v", shutil.EscapeSlice(cmd.Args), err))
		}
		cancel()
	}

	// Also copy crash-related system info (e.g. /etc/lsb-release) to aid in debugging.
	// Having an easy way to see info about the system image (e.g. board name and version) is particularly useful.
	if err := crash.CopySystemInfo(dir); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ", "))
	}
	return nil
}

func suspendLogCleanup() error {
	return ioutil.WriteFile(cleanupLogsPausedPath, nil, 0666)
}

func resumeLogCleanup() error {
	return os.Remove(cleanupLogsPausedPath)
}
