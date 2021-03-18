// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnose

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/testutil"
)

const (
	anotherBootID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
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

func TestReadBootID(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	conn, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	b, err := linuxssh.ReadBootID(context.Background(), conn.SSHConn())
	if err != nil {
		t.Fatal("ReadBootID failed: ", err)
	}
	if b != fakerunner.DefaultBootID {
		t.Errorf("ReadBootID returned %q; want %q", b, fakerunner.DefaultBootID)
	}
}

func TestSSHDropNotRecovered(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Pass a canceled context to make reconnection fail.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msg := SSHDrop(ctx, &td.Cfg, cc, td.TempDir)
	const exp = "target did not come back: context canceled"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropNoReboot(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// boot_id is not changed.

	msg := SSHDrop(context.Background(), &td.Cfg, cc, td.TempDir)
	const exp = "target did not reboot, probably network issue"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropUnknownCrash(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Change boot_id to simulate reboot.
	td.BootID = anotherBootID

	msg := SSHDrop(context.Background(), &td.Cfg, cc, td.TempDir)
	const exp = "target rebooted for unknown crash"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropNormalReboot(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Simulate normal reboot.
	td.BootID = anotherBootID
	td.UnifiedLog = `...
Apr 19 07:12:49 pre-shutdown[31389]: Shutting down for reboot: system-update
...
`

	msg := SSHDrop(context.Background(), &td.Cfg, cc, td.TempDir)
	const exp = "target normally shut down for reboot (system-update)"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropKernelCrashBugX86(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.BootID = anotherBootID
	td.Ramoops = loadTestData(t, "ramoops_crash_x86.txt")

	msg := SSHDrop(context.Background(), &td.Cfg, cc, td.TempDir)
	const exp = "kernel crashed in i915_gem_execbuffer_relocate_vma+0x424/0x757"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropKernelCrashBugARM(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.BootID = anotherBootID
	td.Ramoops = loadTestData(t, "ramoops_crash_arm.txt")

	msg := SSHDrop(context.Background(), &td.Cfg, cc, td.TempDir)
	// TODO(nya): Improve the symbol extraction. In this case, do_raw_spin_lock or
	// spin_bug seems to be a better choice for diagnosis.
	const exp = "kernel crashed in _clear_bit+0x20/0x38"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestSSHDropKernelHungX86(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.BootID = anotherBootID
	td.Ramoops = loadTestData(t, "ramoops_hung_x86.txt")

	msg := SSHDrop(context.Background(), &td.Cfg, cc, td.TempDir)
	const exp = "kernel crashed: kswapd0:32 hung in jbd2_log_wait_commit+0xb9/0x13c"
	if msg != exp {
		t.Errorf("SSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHSaveFiles(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.BootID = anotherBootID
	td.UnifiedLog = "foo"
	td.Ramoops = "bar"

	outDir := filepath.Join(td.TempDir, "diagnosis")
	os.MkdirAll(outDir, 0777)

	SSHDrop(context.Background(), &td.Cfg, cc, outDir)

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
