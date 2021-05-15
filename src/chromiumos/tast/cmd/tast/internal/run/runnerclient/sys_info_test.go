// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
)

// This file uses types and functions from local_test.go.

func TestGetInitialSysInfo(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// Report a few log files and crashes.
	res := jsonprotocol.RunnerGetSysInfoStateResult{
		State: jsonprotocol.SysInfoState{
			LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
			MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
		},
	}
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{Mode: jsonprotocol.RunnerGetSysInfoStateMode})

		json.NewEncoder(stdout).Encode(res)
		return 0
	}

	// Check that the expected command is sent to the DUT and that the returned state is decoded properly.
	td.Cfg.CollectSysInfo = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if err := GetInitialSysInfo(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("GetInitialSysInfo(..., %+v) failed: %v", td.Cfg, err)
	}

	if td.State.InitialSysInfo == nil {
		t.Error("InitialSysInfo is nil")
	} else if !reflect.DeepEqual(*td.State.InitialSysInfo, res.State) {
		t.Errorf("InitialSysInfo is %+v; want %+v", *td.State.InitialSysInfo, res.State)
	}

	// The second call should fail, because it tried to update cfg's field twice.
	if err := GetInitialSysInfo(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Fatal("Calling GetInitialSysInfo twice unexpectedly succeeded")
	}
}

func TestCollectSysInfo(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode:           jsonprotocol.RunnerCollectSysInfoMode,
			CollectSysInfo: &jsonprotocol.RunnerCollectSysInfoArgs{InitialState: *td.State.InitialSysInfo},
		})

		json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerCollectSysInfoResult{})
		return 0
	}

	td.Cfg.CollectSysInfo = true
	td.State.InitialSysInfo = &jsonprotocol.SysInfoState{
		LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
		MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
	}
	ctx := context.Background()
	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(ctx)
	if err := collectSysInfo(ctx, &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("collectSysInfo(..., %+v) failed: %v", td.Cfg, err)
	}

	// TODO(derat): The test SSH server doesn't support file copies. If/when that changes, set the
	// LogDir and CrashDir in the result that returned above and verify that collectSysInfo copies
	// the directories.
}
