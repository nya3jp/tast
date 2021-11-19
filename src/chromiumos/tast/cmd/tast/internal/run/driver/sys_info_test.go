// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/testutil"
)

func TestDriver_CollectSysInfo(t *testing.T) {
	fakeState := &protocol.SysInfoState{
		LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
		MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
	}
	fakeLogs := map[string]string{
		"messages":      "this is syslog",
		"chrome/chrome": "this is chrome log",
	}
	fakeCrashes := map[string]string{
		"chrome.dmp": "chrome crash dump",
		"kernel.dmp": "kernel crash dump",
	}

	env := runtest.SetUp(t,
		runtest.WithGetSysInfoState(func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
			return &protocol.GetSysInfoStateResponse{State: fakeState}, nil
		}),
		runtest.WithCollectSysInfo(func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
			// Ensure SysInfoState matches.
			if diff := cmp.Diff(req.GetInitialState(), fakeState, protocmp.Transform()); diff != "" {
				t.Errorf("CollectSysInfo: InitialState mismatch (-got +want):\n%s", diff)
			}

			// Write fake system logs and crash dumps.
			logDir := testutil.TempDir(t)
			if err := testutil.WriteFiles(logDir, fakeLogs); err != nil {
				t.Fatalf("Failed to write fake logs: %v", err)
			}
			crashDir := testutil.TempDir(t)
			if err := testutil.WriteFiles(crashDir, fakeCrashes); err != nil {
				t.Fatalf("Failed to write fake crashes: %v", err)
			}
			return &protocol.CollectSysInfoResponse{
				LogDir:   logDir,
				CrashDir: crashDir,
			}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	state, err := drv.GetSysInfoState(ctx)
	if err != nil {
		t.Fatalf("GetSysInfoState failed: %v", err)
	}

	if err := drv.CollectSysInfo(ctx, state); err != nil {
		t.Fatalf("CollectSysInfo failed: %v", err)
	}

	gotLogs, err := testutil.ReadFiles(filepath.Join(cfg.ResDir(), driver.SystemLogsDir))
	if err != nil {
		t.Fatalf("Failed to read log dir: %v", err)
	}
	if diff := cmp.Diff(gotLogs, fakeLogs); diff != "" {
		t.Errorf("Logs mismatch (-got +want):\n%s", diff)
	}

	gotCrashes, err := testutil.ReadFiles(filepath.Join(cfg.ResDir(), driver.CrashesDir))
	if err != nil {
		t.Fatalf("Failed to read crash dir: %v", err)
	}
	if diff := cmp.Diff(gotCrashes, fakeCrashes); diff != "" {
		t.Errorf("Crash dumps mismatch (-got +want):\n%s", diff)
	}
}
