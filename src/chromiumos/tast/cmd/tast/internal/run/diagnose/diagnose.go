// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diagnose implements diagnosis logic for run failures.
package diagnose

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/testingutil"
)

// SSHDrop diagnoses a SSH connection drop during local test runs
// and returns a diagnosis message. Files useful for diagnosis might be saved
// under outDir.
func SSHDrop(ctx context.Context, drv *driver.Driver, outDir string) string {
	if drv.InitBootID() == "" {
		return "failed to diagnose: initial boot_id is not available"
	}

	logging.Info(ctx, "Reconnecting to diagnose lost SSH connection")
	const reconnectTimeout = time.Minute
	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		return drv.ReconnectIfNeeded(ctx)
	}, &testingutil.PollOptions{Timeout: reconnectTimeout}); err != nil {
		return fmt.Sprint("target did not come back: ", err)
	}

	// Compare boot_id to see if the target rebooted.
	bootID, err := linuxssh.ReadBootID(ctx, drv.SSHConn())
	if err != nil {
		return fmt.Sprint("failed to diagnose: failed to read boot_id: ", err)
	}

	if bootID == drv.InitBootID() {
		return "target did not reboot, probably network issue"
	}

	// Target rebooted.
	return diagnoseReboot(ctx, drv, outDir)
}

var (
	// shutdownReasonRe matches pre-shutdown message in systemd journal. The first submatch is
	// the type of shutdown (reboot/halt), and the second submatch is the reason.
	shutdownReasonRe = regexp.MustCompile(`pre-shutdown.*Shutting down for (.*): (.*)`)

	// hungRe matches kernel hung log messages. The first submatch is the name of the hung
	// kernel thread, and the second submatch contains its backtrace.
	hungRe = regexp.MustCompile(`(?s)INFO: task ([^\n]+) blocked for more than \d+ seconds\.
.*?Call Trace:
((?:\[[^]]*\]  [^\n]*\n)+).*Kernel panic - not syncing: hung_task: blocked tasks`)

	// crashSymbolRe matches kernel crash log messages. The first submatch is a symbolized
	// function name of the instruction pointer, e.g. "sysrq_handle_crash".
	crashSymbolRe = regexp.MustCompile(`(?:RIP:|PC is at).* (\S+)`)
)

// diagnoseReboot diagnoses the target reboot during local test runs
// and returns a diagnosis message. Files useful for diagnosis might be saved
// under outDir.
func diagnoseReboot(ctx context.Context, drv *driver.Driver, outDir string) string {
	conn := drv.SSHConn()

	// Read the unified system log just before the reboot.
	denseBootID := strings.Replace(drv.InitBootID(), "-", "", -1)
	out, err := conn.Command("croslog", "--quiet", "--boot="+denseBootID, "--lines=1000").Output(ctx)
	if err != nil {
		logging.Info(ctx, "Failed to execute croslog command: ", err)
		out, err = conn.Command("journalctl", "--quiet", "--boot="+denseBootID, "--lines=1000").Output(ctx)
		if err != nil {
			logging.Info(ctx, "Failed to execute journalctl command: ", err)
		} else {
			logging.Info(ctx, "Fell back to journalctl command")
		}
	}
	logs := string(out)

	if logs != "" {
		const fn = "unified-logs.before-reboot.txt"
		if err := ioutil.WriteFile(filepath.Join(outDir, fn), []byte(logs), 0666); err != nil {
			logging.Infof(ctx, "Failed to save %s: %v", fn, err)
		}
	}

	// Read console-ramoops. Its path varies by systems, and it might not exist
	// for normal reboots.
	out, err = conn.Command("cat", "/sys/fs/pstore/console-ramoops-0").Output(ctx)
	if err != nil {
		out, err = conn.Command("cat", "/sys/fs/pstore/console-ramoops").Output(ctx)
		if err != nil {
			logging.Info(ctx, "console-ramoops not found")
		}
	}
	ramOops := string(out)

	if ramOops != "" {
		const fn = "console-ramoops.txt"
		if err := ioutil.WriteFile(filepath.Join(outDir, fn), []byte(ramOops), 0666); err != nil {
			logging.Infof(ctx, "Failed to save %s: %v", fn, err)
		}
	}

	if m := shutdownReasonRe.FindStringSubmatch(logs); m != nil {
		return fmt.Sprintf("target normally shut down for %s (%s)", m[1], m[2])
	}

	if m := hungRe.FindStringSubmatch(ramOops); m != nil {
		proc := m[1]
		for _, line := range strings.Split(m[2], "\n") {
			fs := strings.Fields(line)
			if len(fs) == 0 {
				continue
			}
			f := fs[len(fs)-1]
			// Skip schedule functions.
			if strings.Contains(f, "schedule") {
				continue
			}
			return fmt.Sprintf("kernel crashed: %s hung in %s", proc, f)
		}
		return fmt.Sprintf("kernel crashed: %s hung", proc)
	}

	if ms := crashSymbolRe.FindAllStringSubmatch(ramOops, -1); ms != nil {
		m := ms[len(ms)-1]
		return fmt.Sprint("kernel crashed in ", m[1])
	}

	return "target rebooted for unknown crash"
}
