// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/internal/crash"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/logs"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/shutil"
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

// GetSysInfoState implements the GetSysInfoState RPC method for ChromeOS.
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

	crashFilePaths, err := getCrashFilePaths()
	if err != nil {
		return nil, err
	}

	return &protocol.GetSysInfoStateResponse{
		State: &protocol.SysInfoState{
			LogInodeSizes:    logInodeSizes,
			UnifiedLogCursor: unifiedLogCursor,
			CrashFilePaths:   crashFilePaths,
		},
	}, nil
}

// CollectSysInfo implements the CollectSysInfo RPC method for ChromeOS.
func CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
	initialState := req.GetInitialState()

	// Collect syslog logs.
	logDir, err := os.MkdirTemp("", "tast_logs.")
	if err != nil {
		return nil, errors.Wrap(err, "failed making tast_logs temp dir")
	}
	if err := logs.CopyLogFileUpdates(ctx, systemLogDir, logDir, systemLogExcludes, initialState.GetLogInodeSizes()); err != nil {
		return nil, errors.Wrap(err, "failed CopyLogFileUpdates to tast_logs")
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
	dumps, err := getCrashFilePaths()
	if err != nil {
		return nil, errors.Wrap(err, "failed getCrashFilePaths")
	}
	crashDir, err := os.MkdirTemp("", "tast_crashes.")
	if err != nil {
		return nil, errors.Wrap(err, "failed making tast_crashes temp dir")
	}
	if err := crash.CopyNewFiles(ctx, crashDir, dumps, initialState.GetCrashFilePaths()); err != nil {
		return nil, errors.Wrap(err, "failed CopyNewFiles to tast_crashes")
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

// getCrashFilePaths returns the paths of all minidump files, meta files
// and all files with a meta file prefix within dirs.
func getCrashFilePaths() ([]string, error) {
	var dumps []string
	var metaFilePrefixes []string
	paths, err := crash.GetCrashes(crash.DefaultDirs()...)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if filepath.Ext(path) == crash.MetadataExt {
			prefix := strings.TrimSuffix(path, crash.MetadataExt)
			metaFilePrefixes = append(metaFilePrefixes, prefix)
		}
	}
	for _, path := range paths {
		if filepath.Ext(path) == crash.MinidumpExt {
			dumps = append(dumps, path)
			continue
		}
		for _, prefix := range metaFilePrefixes {
			if strings.HasPrefix(path, prefix) {
				dumps = append(dumps, path)
				// The current path match the current prefix so we can skip other prefixes.
				break
			}
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
		"ps.txt":           {"sh", "-c", "ps auxwwf || ps -Afw"},
		"mount.txt":        {"mount"},
		"hostname.txt":     {"hostname"},
		"uptime.txt":       {"uptime"},
		"losetup.txt":      {"sh", "-c", "losetup || losetup -a"},
		"lscpu.txt":        {"lscpu"},
		"df.txt":           {"df", "-kP"},
		"lvs.txt":          {"lvs", "-a", "--units=m"},
		"dmesg.txt":        {"dmesg"},
	}
	if _, err := os.Stat("/mnt/stateful_partition"); !os.IsNotExist(err) {
		cmds["du_stateful.txt"] = []string{"du", "-m", "/mnt/stateful_partition"}
	}
	if _, err := os.Stat("/data"); !os.IsNotExist(err) {
		cmds["du_data.txt"] = []string{"du", "-m", "/data"}
	}
	if _, err := os.Stat("/proc/bus/pci"); !os.IsNotExist(err) {
		cmds["lspci.txt"] = []string{"sh", "-c", "lspci -vvnn || lspci -nn"}
	}
	if _, err := os.Stat("/sys/bus/usb"); !os.IsNotExist(err) {
		cmds["lsusb.txt"] = []string{"sh", "-c", "lsusb -vt || lsusb"}
	}

	for fn, cmd := range cmds {
		// Set timeout in case some commands take long time unexpectedly. (crbug.com/1147723)
		cmdCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
		cmd := exec.CommandContext(cmdCtx, cmd[0], cmd[1:]...)
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

	// Copy notable system log files which are not expected to change during the test.
	staticLogs := []string{
		"ec_info.txt",
		"bios_info.txt",
	}
	for _, staticLog := range staticLogs {
		staticLogPath := filepath.Join(systemLogDir, staticLog)
		if _, err := os.Stat(staticLogPath); !os.IsNotExist(err) {
			fsutil.CopyFile(staticLogPath, filepath.Join(dir, staticLog))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ", "))
	}
	return nil
}

func suspendLogCleanup() error {
	return os.WriteFile(cleanupLogsPausedPath, nil, 0666)
}

func resumeLogCleanup() error {
	return os.Remove(cleanupLogsPausedPath)
}
