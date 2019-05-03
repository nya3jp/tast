// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/host"
	"chromiumos/tast/testing"
)

// readBootID reads the current boot_id at hst.
func readBootID(ctx context.Context, hst *host.SSH) (string, error) {
	out, err := hst.Run(ctx, "cat /proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// diagnoseSSHDrop diagnoses a SSH connection drop during local test runs
// and returns a diagnosis message.
func diagnoseSSHDrop(ctx context.Context, cfg *Config) string {
	if cfg.initBootID == "" {
		return "failed to diagnose: initial boot_id is not available"
	}

	cfg.Logger.Log("Reconnecting to diagnose lost SSH connection")
	const reconnectTimeout = time.Minute
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := connectToTarget(ctx, cfg)
		return err
	}, &testing.PollOptions{Timeout: reconnectTimeout}); err != nil {
		return fmt.Sprint("target did not come back: ", err)
	}

	// Compare boot_id to see if the target rebooted.
	bootID, err := readBootID(ctx, cfg.hst)
	if err != nil {
		return fmt.Sprint("failed to diagnose: failed to read boot_id: ", err)
	}

	if bootID == cfg.initBootID {
		return "target did not reboot, probably network issue"
	}

	// Target rebooted.
	return diagnoseReboot(ctx, cfg)
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
// and returns a diagnosis message.
func diagnoseReboot(ctx context.Context, cfg *Config) string {
	// Read the journal just before the reboot.
	denseBootID := strings.Replace(cfg.initBootID, "-", "", -1)
	cmd := fmt.Sprintf("journalctl -q -b %s -n 1000", denseBootID)
	out, err := cfg.hst.Run(ctx, cmd)
	if err != nil {
		cfg.Logger.Log("Failed to read journal: ", err)
	}
	journal := string(out)

	// Read console-ramoops. Its path varies by systems, and it might not exist
	// for normal reboots.
	out, err = cfg.hst.Run(ctx, "cat /sys/fs/pstore/console-ramoops-0")
	if err != nil {
		out, err = cfg.hst.Run(ctx, "cat /sys/fs/pstore/console-ramoops")
		if err != nil {
			cfg.Logger.Log("console-ramoops not found")
		}
	}
	ramOops := string(out)

	if m := shutdownReasonRe.FindStringSubmatch(journal); m != nil {
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
