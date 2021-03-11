// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

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
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	hst, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	b, err := linuxssh.ReadBootID(context.Background(), hst)
	if err != nil {
		t.Fatal("ReadBootID failed: ", err)
	}
	if b != defaultBootID {
		t.Errorf("ReadBootID returned %q; want %q", b, defaultBootID)
	}
}

func TestDiagnoseSSHDropNotRecovered(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Pass a canceled context to make reconnection fail.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msg := diagnoseSSHDrop(ctx, &td.cfg, cc, td.tempDir)
	const exp = "target did not come back: context canceled"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropNoReboot(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// boot_id is not changed.

	msg := diagnoseSSHDrop(context.Background(), &td.cfg, cc, td.tempDir)
	const exp = "target did not reboot, probably network issue"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropUnknownCrash(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Change boot_id to simulate reboot.
	td.bootID = anotherBootID

	msg := diagnoseSSHDrop(context.Background(), &td.cfg, cc, td.tempDir)
	const exp = "target rebooted for unknown crash"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropNormalReboot(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Simulate normal reboot.
	td.bootID = anotherBootID
	td.unifiedLog = `...
Apr 19 07:12:49 pre-shutdown[31389]: Shutting down for reboot: system-update
...
`

	msg := diagnoseSSHDrop(context.Background(), &td.cfg, cc, td.tempDir)
	const exp = "target normally shut down for reboot (system-update)"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropKernelCrashBugX86(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.bootID = anotherBootID
	td.ramOops = loadTestData(t, "ramoops_crash_x86.txt")

	msg := diagnoseSSHDrop(context.Background(), &td.cfg, cc, td.tempDir)
	const exp = "kernel crashed in i915_gem_execbuffer_relocate_vma+0x424/0x757"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropKernelCrashBugARM(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.bootID = anotherBootID
	td.ramOops = loadTestData(t, "ramoops_crash_arm.txt")

	msg := diagnoseSSHDrop(context.Background(), &td.cfg, cc, td.tempDir)
	// TODO(nya): Improve the symbol extraction. In this case, do_raw_spin_lock or
	// spin_bug seems to be a better choice for diagnosis.
	const exp = "kernel crashed in _clear_bit+0x20/0x38"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropKernelHungX86(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.bootID = anotherBootID
	td.ramOops = loadTestData(t, "ramoops_hung_x86.txt")

	msg := diagnoseSSHDrop(context.Background(), &td.cfg, cc, td.tempDir)
	const exp = "kernel crashed: kswapd0:32 hung in jbd2_log_wait_commit+0xb9/0x13c"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHSaveFiles(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := cc.Conn(context.Background()); err != nil {
		t.Fatal(err)
	}

	td.bootID = anotherBootID
	td.unifiedLog = "foo"
	td.ramOops = "bar"

	outDir := filepath.Join(td.tempDir, "diagnosis")
	os.MkdirAll(outDir, 0777)

	diagnoseSSHDrop(context.Background(), &td.cfg, cc, outDir)

	files, err := testutil.ReadFiles(outDir)
	if err != nil {
		t.Fatal("ReadFiles failed: ", err)
	}
	exp := map[string]string{
		"unified-logs.before-reboot.txt": "foo",
		"console-ramoops.txt":            "bar",
	}
	if diff := cmp.Diff(files, exp); diff != "" {
		t.Error("diagnoseSSHDrop did not save files as expected (-got +want):\n", diff)
	}
}
