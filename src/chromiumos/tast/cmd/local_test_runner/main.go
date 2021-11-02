// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the local_test_runner executable.
//
// local_test_runner is executed on-device by the tast command.
// It runs test bundles and reports the results back to tast.
// It is also used to query additional information about the DUT
// such as logs, crashes, and supported software features.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/internal/crash"
	"chromiumos/tast/internal/crosbundle"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/shutil"
)

func main() {
	args := jsonprotocol.RunnerArgs{
		RunTests: &jsonprotocol.RunnerRunTestsArgs{
			BundleGlob: "/usr/local/libexec/tast/bundles/local/*",
			BundleArgs: jsonprotocol.BundleRunTestsArgs{
				DataDir: "/usr/local/share/tast/data",
				TempDir: "/usr/local/tmp/tast/run_tmp",
			},
		},
	}
	scfg := runner.StaticConfig{
		Type:                               runner.LocalRunner,
		KillStaleRunners:                   true,
		EnableSyslog:                       true,
		SystemLogDir:                       "/var/log",
		SystemLogExcludes:                  []string{"journal"}, // journald binary logs: https://crbug.com/931951
		UnifiedLogSubdir:                   "unified",           // destination for exported unified system logs
		SystemInfoFunc:                     writeSystemInfo,     // save additional system info at end of run
		SystemCrashDirs:                    crash.DefaultDirs(),
		CleanupLogsPausedPath:              "/var/lib/cleanup_logs_paused",
		GetDUTInfo:                         crosbundle.GetDUTInfo,
		DeprecatedDefaultBuildArtifactsURL: crosbundle.DeprecatedDefaultBuildArtifactsURL,
		PrivateBundlesStampPath:            "/usr/local/share/tast/.private-bundles-downloaded",
	}
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &args, &scfg))
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
