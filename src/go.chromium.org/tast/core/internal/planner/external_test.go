// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"strings"
	"sync/atomic"
	gotesting "testing"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/internal/minidriver/target"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/internal/testing"
)

// externalConnCacheTestEnv holds a fake DUT for newExternalConnCache tests.
type externalConnCacheTestEnv struct {
	tcfg *target.Config
	rd   *testing.RemoteData
	// bootIDFailures is the number of the next boot ID reads to fail.
	bootIDFailures atomic.Int32
}

// setUpExternalConnCacheTest starts a fake DUT running commands locally, and
// returns a target.Config for it and RemoteData connected to it.
func setUpExternalConnCacheTest(ctx context.Context, t *gotesting.T) *externalConnCacheTestEnv {
	t.Helper()
	env := &externalConnCacheTestEnv{}
	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		if strings.Contains(req.Cmd, "boot_id") && env.bootIDFailures.Add(-1) >= 0 {
			req.Start(false)
			return
		}
		req.Start(true)
		req.End(req.RunRealCmd())
	})
	t.Cleanup(td.Close)

	spec := td.Srvs[0].Addr().String()
	env.tcfg = &target.Config{
		SSHConfig:     &protocol.SSHConfig{ConnectionSpec: spec, KeyFile: td.UserKeyFile},
		ServiceConfig: &target.ServiceConfig{},
	}
	dt, err := dut.New(spec, td.UserKeyFile, "", "", nil)
	if err != nil {
		t.Fatal("dut.New failed: ", err)
	}
	if err := dt.Connect(ctx); err != nil {
		t.Fatal("DUT.Connect failed: ", err)
	}
	t.Cleanup(func() { dt.Close(ctx) })
	env.rd = &testing.RemoteData{Meta: &testing.Meta{ConnectionSpec: spec}, DUT: dt}
	return env
}

func TestNewExternalConnCache(t *gotesting.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name             string
		setUp            func(env *externalConnCacheTestEnv) error
		wantShared       bool
		wantDUTConnected bool
	}{
		{
			name:             "shares_dut_connection",
			setUp:            func(*externalConnCacheTestEnv) error { return nil },
			wantShared:       true,
			wantDUTConnected: true,
		},
		{
			name: "no_remote_data",
			setUp: func(env *externalConnCacheTestEnv) error {
				env.rd = nil
				return nil
			},
			wantDUTConnected: true,
		},
		{
			name: "no_meta",
			setUp: func(env *externalConnCacheTestEnv) error {
				env.rd.Meta = nil
				return nil
			},
			wantDUTConnected: true,
		},
		{
			name: "no_dut",
			setUp: func(env *externalConnCacheTestEnv) error {
				env.rd.DUT = nil
				return nil
			},
			wantDUTConnected: true,
		},
		{
			name: "other_target",
			setUp: func(env *externalConnCacheTestEnv) error {
				env.rd.Meta.ConnectionSpec = "other:22"
				return nil
			},
			wantDUTConnected: true,
		},
		{
			name: "disconnected_dut",
			setUp: func(env *externalConnCacheTestEnv) error {
				return env.rd.DUT.Disconnect(ctx)
			},
		},
		{
			// DUT.Connected pings the stale connection.
			name: "broken_connection",
			setUp: func(env *externalConnCacheTestEnv) error {
				return env.rd.DUT.Conn().Close(ctx)
			},
		},
		{
			name: "sharing_fails",
			setUp: func(env *externalConnCacheTestEnv) error {
				env.bootIDFailures.Store(1)
				return nil
			},
			wantDUTConnected: true,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			env := setUpExternalConnCacheTest(ctx, t)
			dt := env.rd.DUT
			if err := tc.setUp(env); err != nil {
				t.Fatal("setUp failed: ", err)
			}

			cc, err := newExternalConnCache(ctx, env.tcfg, env.rd)
			if err != nil {
				t.Fatal("newExternalConnCache failed: ", err)
			}
			if shared := cc.Conn().SSHConn() == dt.Conn(); shared != tc.wantShared {
				t.Errorf("Shared the DUT connection: %v; want %v", shared, tc.wantShared)
			}
			if err := cc.Close(ctx); err != nil {
				t.Error("Close failed: ", err)
			}
			if connected := dt.Connected(ctx); connected != tc.wantDUTConnected {
				t.Errorf("DUT connected after Close: %v; want %v", connected, tc.wantDUTConnected)
			}
		})
	}
}
