// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"chromiumos/tast/runner"
)

// This file uses types and functions from local_test.go.

func TestGetInitialSysInfo(t *testing.T) {
	td := newLocalTestData()
	defer td.close()

	// Report a few log files and crashes.
	ob := bytes.Buffer{}
	res := runner.GetSysInfoStateResult{
		State: runner.SysInfoState{
			LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
			MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
		},
	}
	if err := json.NewEncoder(&ob).Encode(&res); err != nil {
		t.Fatal(err)
	}
	stdin := addLocalRunnerFakeCmd(td.srvData.Srv, 0, ob.Bytes(), nil)

	// Check that the expected command is sent to the DUT and that the returned state is decoded properly.
	td.cfg.CollectSysInfo = true
	if err := getInitialSysInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getInitialSysInfo(..., %+v) failed: %v", td.cfg, err)
	}
	checkArgs(t, stdin, &runner.Args{Mode: runner.GetSysInfoStateMode})

	if td.cfg.initialSysInfo == nil {
		t.Error("initialSysInfo is nil")
	} else if !reflect.DeepEqual(*td.cfg.initialSysInfo, res.State) {
		t.Errorf("initialSysInfo is %+v; want %+v", *td.cfg.initialSysInfo, res.State)
	}

	// After a second call, the initial state should be left unchanged
	// (and a request shouldn't even be sent to the DUT).
	if err := getInitialSysInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("no-op getInitialSysInfo(..., %+v) failed: %v", td.cfg, err)
	}
	if !reflect.DeepEqual(*td.cfg.initialSysInfo, res.State) {
		t.Errorf("updated initialSysInfo is %+v; want %+v", *td.cfg.initialSysInfo, res.State)
	}
}

func TestCollectSysInfo(t *testing.T) {
	td := newLocalTestData()
	defer td.close()

	ob := bytes.Buffer{}
	if err := json.NewEncoder(&ob).Encode(&runner.CollectSysInfoResult{}); err != nil {
		t.Fatal(err)
	}
	stdin := addLocalRunnerFakeCmd(td.srvData.Srv, 0, ob.Bytes(), nil)

	td.cfg.CollectSysInfo = true
	td.cfg.initialSysInfo = &runner.SysInfoState{
		LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
		MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
	}
	if err := collectSysInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("collectSysInfo(..., %+v) failed: %v", td.cfg, err)
	}
	checkArgs(t, stdin, &runner.Args{
		Mode:               runner.CollectSysInfoMode,
		CollectSysInfoArgs: runner.CollectSysInfoArgs{InitialState: *td.cfg.initialSysInfo},
	})

	// TODO(derat): The test SSH server doesn't support file copies. If/when that changes, set the
	// LogDir and CrashDir in the result that returned above and verify that collectSysInfo copies
	// the directories.
}
