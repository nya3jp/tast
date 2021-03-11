// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
)

const (
	// SSHPingTimeout is the timeout for checking if SSH connection to DUT is open.
	SSHPingTimeout = 5 * time.Second

	sshConnectTimeout = 10 * time.Second // timeout for establishing SSH connection to DUT
	sshRetryInterval  = 5 * time.Second  // minimum time to wait between SSH connection attempts
)

func healthy(ctx context.Context, conn *ssh.Conn) error {
	if err := conn.Ping(ctx, SSHPingTimeout); err != nil {
		return errors.Wrap(err, "target connection is broken")
	}
	return nil
}

func dialSSH(ctx context.Context, cfg *config.Config) (*ssh.Conn, error) {
	ctx, st := timing.Start(ctx, "connect")
	defer st.End()
	cfg.Logger.Logf("Connecting to %s", cfg.Target)

	opts := &ssh.Options{
		ConnectTimeout:       sshConnectTimeout,
		ConnectRetries:       cfg.SSHRetries,
		ConnectRetryInterval: sshRetryInterval,
		KeyFile:              cfg.KeyFile,
		KeyDir:               cfg.KeyDir,
		WarnFunc:             func(s string) { cfg.Logger.Log(s) },
	}
	if err := ssh.ParseTarget(cfg.Target, opts); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, sshConnectTimeout*time.Duration(cfg.SSHRetries+1))
	defer cancel()

	conn, err := ssh.New(ctx, opts)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
