// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/runner"
)

// This file uses types and functions from local_test.go.

func TestGetInitialSysInfo(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// Report a few log files and crashes.
	res := runner.GetSysInfoStateResult{
		State: runner.SysInfoState{
			LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
			MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
		},
	}
	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{Mode: runner.GetSysInfoStateMode})

		json.NewEncoder(stdout).Encode(res)
		return 0
	}

	// Check that the expected command is sent to the DUT and that the returned state is decoded properly.
	td.Cfg.CollectSysInfo = true

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if err := getInitialSysInfo(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("getInitialSysInfo(..., %+v) failed: %v", td.Cfg, err)
	}

	if td.State.InitialSysInfo == nil {
		t.Error("InitialSysInfo is nil")
	} else if !reflect.DeepEqual(*td.State.InitialSysInfo, res.State) {
		t.Errorf("InitialSysInfo is %+v; want %+v", *td.State.InitialSysInfo, res.State)
	}

	// The second call should fail, because it tried to update cfg's field twice.
	if err := getInitialSysInfo(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Fatal("Calling getInitialSysInfo twice unexpectedly succeeded")
	}
}

func TestCollectSysInfo(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:           runner.CollectSysInfoMode,
			CollectSysInfo: &runner.CollectSysInfoArgs{InitialState: *td.State.InitialSysInfo},
		})

		json.NewEncoder(stdout).Encode(&runner.CollectSysInfoResult{})
		return 0
	}

	td.Cfg.CollectSysInfo = true
	td.State.InitialSysInfo = &runner.SysInfoState{
		LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
		MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
	}
	if err := collectSysInfo(context.Background(), &td.Cfg, &td.State); err != nil {
		t.Fatalf("collectSysInfo(..., %+v) failed: %v", td.Cfg, err)
	}

	// TODO(derat): The test SSH server doesn't support file copies. If/when that changes, set the
	// LogDir and CrashDir in the result that returned above and verify that collectSysInfo copies
	// the directories.
}
