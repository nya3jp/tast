// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/minidriver/diagnose"
	"chromiumos/tast/testutil"
)

func loadTestData(t *testing.T, filename string) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal("Getwd failed: ", err)
	}
	b, err := ioutil.ReadFile(filepath.Join(wd, "testdata", filename))
	if err != nil {
		t.Fatal("ReadFile failed: ", err)
	}
	return string(b)
}

func callSSHDrop(t *testing.T, rebooted bool, syslog, ramoops string) (msg, outDir string) {
	currentBootID := "11111111"
	env := runtest.SetUp(t,
		runtest.WithBootID(func() (bootID string, err error) {
			return currentBootID, nil
		}),
		runtest.WithExtraSSHHandlers([]fakesshserver.Handler{
			fakesshserver.ExactMatchHandler("exec croslog --quiet --boot=11111111 --lines=1000", func(_ io.Reader, stdout, _ io.Writer) int {
				io.WriteString(stdout, syslog)
				return 0
			}),
			fakesshserver.ExactMatchHandler("exec cat /sys/fs/pstore/console-ramoops-0", func(_ io.Reader, stdout, _ io.Writer) int {
				io.WriteString(stdout, ramoops)
				return 0
			}),
		}),
	)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if rebooted {
		currentBootID = "22222222" // pretend rebooted
	}

	outDir = filepath.Join(env.TempDir(), "diagnose")
	if err := os.MkdirAll(outDir, 0777); err != nil {
		t.Fatal(err)
	}

	msg = diagnose.SSHDrop(ctx, drv.ConnCacheForTesting(), outDir)
	return msg, outDir
}

func TestSSHDropNotRecovered(t *testing.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	// Pass a canceled context to make reconnection fail.
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	msg := diagnose.SSHDrop(ctx, drv.ConnCacheForTesting(), env.TempDir())
	const exp = "target did not come back: context canceled"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropNoReboot(t *testing.T) {
	msg, _ := callSSHDrop(t, false, "", "")
	const exp = "target did not reboot, probably network issue"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropUnknownCrash(t *testing.T) {
	msg, _ := callSSHDrop(t, true, "", "")
	const exp = "target rebooted for unknown crash"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropNormalReboot(t *testing.T) {
	const syslog = `...
Apr 19 07:12:49 pre-shutdown[31389]: Shutting down for reboot: system-update
...
`
	msg, _ := callSSHDrop(t, true, syslog, "")
	const exp = "target normally shut down for reboot (system-update)"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropKernelCrashBugX86(t *testing.T) {
	ramoops := loadTestData(t, "ramoops_crash_x86.txt")
	msg, _ := callSSHDrop(t, true, "", ramoops)
	const exp = "kernel crashed in i915_gem_execbuffer_relocate_vma+0x424/0x757"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropKernelCrashBugARM(t *testing.T) {
	ramoops := loadTestData(t, "ramoops_crash_arm.txt")
	msg, _ := callSSHDrop(t, true, "", ramoops)
	// TODO(nya): Improve the symbol extraction. In this case, do_raw_spin_lock or
	// spin_bug seems to be a better choice for diagnosis.
	const exp = "kernel crashed in _clear_bit+0x20/0x38"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropKernelHungX86(t *testing.T) {
	ramoops := loadTestData(t, "ramoops_hung_x86.txt")
	msg, _ := callSSHDrop(t, true, "", ramoops)
	const exp = "kernel crashed: kswapd0:32 hung in jbd2_log_wait_commit+0xb9/0x13c"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHSaveFiles(t *testing.T) {
	const (
		syslog  = "foo"
		ramoops = "bar"
	)

	_, outDir := callSSHDrop(t, true, syslog, ramoops)

	files, err := testutil.ReadFiles(outDir)
	if err != nil {
		t.Fatal("ReadFiles failed: ", err)
	}
	exp := map[string]string{
		"unified-logs.before-reboot.txt": "foo",
		"console-ramoops.txt":            "bar",
	}
	if diff := cmp.Diff(files, exp); diff != "" {
		t.Error("SSHDrop did not save files as expected (-got +want):\n", diff)
	}
}
