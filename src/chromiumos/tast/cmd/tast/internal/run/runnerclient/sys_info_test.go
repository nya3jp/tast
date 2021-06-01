// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/protocol"
)

func TestGetInitialSysInfo(t *testing.T) {
	ctx := context.Background()

	wantState := &protocol.SysInfoState{
		LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
		MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
	}
	env := runtest.SetUp(t, runtest.WithGetSysInfoState(func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
		return &protocol.GetSysInfoStateResponse{State: wantState}, nil
	}))
	cfg := env.Config()

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(ctx)

	gotState, err := GetInitialSysInfo(context.Background(), cfg, cc)
	if err != nil {
		t.Fatalf("GetInitialSysInfo failed: %v", err)
	}

	if diff := cmp.Diff(gotState, wantState); diff != "" {
		t.Errorf("InitialSysInfoState mismatch (-got +want):\n%s", diff)
	}
}

func TestCollectSysInfo(t *testing.T) {
	ctx := context.Background()

	initialState := &protocol.SysInfoState{
		LogInodeSizes: map[uint64]int64{1: 2, 3: 4},
		MinidumpPaths: []string{"foo.dmp", "bar.dmp"},
	}
	called := false
	env := runtest.SetUp(t, runtest.WithCollectSysInfo(func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
		called = true
		if diff := cmp.Diff(req.GetInitialState(), initialState); diff != "" {
			t.Errorf("CollectSysInfo: InitialState mismatch (-got +want):\n%s", diff)
		}
		return &protocol.CollectSysInfoResponse{}, nil
	}))
	cfg := env.Config()

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(ctx)

	if err := collectSysInfo(ctx, cfg, initialState, cc); err != nil {
		t.Fatalf("collectSysInfo failed: %v", err)
	}
	if !called {
		t.Error("CollectSysInfo was not called")
	}

	// TODO(derat): The test SSH server doesn't support file copies. If/when that changes, set the
	// LogDir and CrashDir in the result that returned above and verify that collectSysInfo copies
	// the directories.
}
