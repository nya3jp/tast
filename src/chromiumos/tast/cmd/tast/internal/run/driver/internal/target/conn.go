// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
)

const (
	// SSHPingTimeout is the timeout for checking if SSH connection to DUT is open.
	SSHPingTimeout = 5 * time.Second

	sshConnectTimeout = 10 * time.Second // timeout for establishing SSH connection to DUT
	sshRetryInterval  = 5 * time.Second  // minimum time to wait between SSH connection attempts
)

// Conn holds an SSH connection to a target device, as well as several other
// objects whose lifetime is tied to the connection such as SSH port forwards.
type Conn struct {
	sshConn *ssh.Conn
	svcs    *Services
	target  string
}

// Config contains configs for creating connection.
type Config struct {
	SSHConfig     *protocol.SSHConfig
	Retries       int
	TastVars      map[string]string
	ServiceConfig *ServiceConfig
}

func newConn(ctx context.Context, cfg *Config, target, dutServer string) (conn *Conn, retErr error) {
	sshConn, err := dialSSH(ctx, cfg.SSHConfig, target, cfg.Retries)
	if err != nil {
		msg, diagErr := diagnoseNetwork(ctx, target)
		if diagErr != nil {
			logging.Info(ctx, "Failed to run network diagnosis: ", diagErr)
			return nil, err
		}
		if msg == "" {
			msg = "no issues found"
		}
		return nil, errors.Errorf("%v (network diagnosis: %s)", err, msg)
	}
	defer func() {
		if retErr != nil {
			sshConn.Close(ctx)
		}
	}()
	svcs, err := startServices(ctx, cfg.ServiceConfig, sshConn, dutServer)
	if err != nil {
		return nil, err
	}

	return &Conn{
		sshConn: sshConn,
		svcs:    svcs,
		target:  target,
	}, nil
}

func (c *Conn) close(ctx context.Context) error {
	var firstErr error
	if err := c.svcs.close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.sshConn.Close(ctx); err != nil {
		firstErr = err
	}
	return firstErr
}

// SSHConn returns an SSH connection. A returned connection is owned by Conn
// and should not be closed by users.
func (c *Conn) SSHConn() *ssh.Conn { return c.sshConn }

// Services returns an object owning services tied to an SSH connection.
func (c *Conn) Services() *Services { return c.svcs }

// Healthy checks health of the connection by sending an SSH ping packet.
func (c *Conn) Healthy(ctx context.Context) error {
	if err := c.sshConn.Ping(ctx, SSHPingTimeout); err != nil {
		return errors.Wrap(err, "target connection is broken")
	}
	return nil
}

// Target returns a connection spec as [<user>@]host[:<port>].
func (c *Conn) Target() string {
	return c.target
}

// dialSSH uses ssh to connect to target which in the format host:port.
func dialSSH(ctx context.Context, cfg *protocol.SSHConfig, target string, retries int) (*ssh.Conn, error) {
	ctx, st := timing.Start(ctx, "connect")
	defer st.End()
	logging.Infof(ctx, "Connecting to %s", target)

	opts := &ssh.Options{
		ConnectTimeout:       sshConnectTimeout,
		ConnectRetries:       retries,
		ConnectRetryInterval: sshRetryInterval,
		KeyFile:              cfg.GetKeyFile(),
		KeyDir:               cfg.GetKeyDir(),
		WarnFunc:             func(s string) { logging.Info(ctx, s) },
	}
	if err := ssh.ParseTarget(target, opts); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, sshConnectTimeout*time.Duration(retries+1))
	defer cancel()

	conn, err := ssh.New(ctx, opts)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
