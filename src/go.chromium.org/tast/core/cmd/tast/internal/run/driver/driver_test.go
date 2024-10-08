// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"io"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/cmd/tast/internal/run/runtest"
	"go.chromium.org/tast/core/internal/fakesshserver"
)

func TestDriver(t *testing.T) {
	const (
		fakeBootID  = "bootID"
		fakeCommand = "fake_command"
	)

	env := runtest.SetUp(
		t,
		runtest.WithBootID(func() (string, error) {
			return fakeBootID, nil
		}),
		runtest.WithExtraSSHHandlers([]fakesshserver.Handler{
			fakesshserver.ExactMatchHandler("exec "+fakeCommand, func(_ io.Reader, _, _ io.Writer) int {
				return 0
			}),
		}),
	)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer func() {
		if err := drv.Close(ctx); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// TODO: Mock SSH config resolution. If localhost is in the config, this
	// check can fail.
	if spec := drv.ConnectionSpec(); spec != cfg.Target() {
		t.Errorf("ConnectionSpec() = %q; want %q", spec, cfg.Target())
	}
	if bootID := drv.InitBootID(); bootID != fakeBootID {
		t.Errorf("InitBootID() = %q; want %q", bootID, fakeBootID)
	}
	if err := drv.SSHConn().CommandContext(ctx, fakeCommand).Run(); err != nil {
		t.Errorf("Command(%q).Run() failed: %v", fakeCommand, err)
	}
}

func TestDriver_RemoteDevservers(t *testing.T) {
	const (
		fakeBootID  = "bootID"
		fakeCommand = "fake_command"
	)

	expectedRemoteDevservers := []string{"devserver1", "devserver2"}

	env := runtest.SetUp(
		t,
		runtest.WithBootID(func() (string, error) {
			return fakeBootID, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", expectedRemoteDevservers)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer func() {
		if err := drv.Close(ctx); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	if !cmp.Equal(drv.RemoteDevservers(), expectedRemoteDevservers) {
		t.Errorf("remoteDevservers = %v; want %v", drv.RemoteDevservers(), expectedRemoteDevservers)
	}
}

func TestDriver_ReconnectIfNeeded(t *testing.T) {
	const pingTimeout = 10 * time.Second
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(nil)

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer func() {
		if err := drv.Close(ctx); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Ping should work initially.
	if err := drv.SSHConn().Ping(ctx, pingTimeout); err != nil {
		t.Fatalf("First Ping failed: %v", err)
	}

	// Forcibly close the current connection. Now Ping fails.
	if err := drv.SSHConn().Close(ctx); err != nil {
		t.Fatalf("ssh.Conn.Close failed: %v", err)
	}
	if err := drv.SSHConn().Ping(ctx, pingTimeout); err == nil {
		t.Fatal("Ping unexpectedly succeeded despite forced close")
	}

	// Reconnect to the target device. Now Ping starts to pass.
	if err := drv.ReconnectIfNeeded(ctx, true, false); err != nil {
		t.Fatalf("ReconnectIfNeeded failed: %v", err)
	}
	if err := drv.SSHConn().Ping(ctx, pingTimeout); err != nil {
		t.Fatalf("Ping failed after reconnection: %v", err)
	}
}
