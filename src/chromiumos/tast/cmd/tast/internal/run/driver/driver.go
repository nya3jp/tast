// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package driver implements communications with local/remote executables
// related to Tast.
package driver

import (
	"context"
	"fmt"
	"os"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/target"
	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/ssh"
)

const (
	// SSHPingTimeout is the timeout for checking if SSH connection to DUT is open.
	SSHPingTimeout = target.SSHPingTimeout
)

// Services owns services exposed to a target device by SSH port forwarding.
type Services = target.Services

// Driver implements communications with local/remote executables related to
// Tast.
//
// Driver maintains a connection to the target device. Getter methods related to
// a current connection are guaranteed to return immediately. Other methods may
// re-establish a connection to the target device, so you should get a fresh
// connection object after calling them.
type Driver struct {
	cfg *config.Config
	cc  *target.ConnCache

	conn *target.Conn // always non-nil
}

// New establishes a new connection to the target device and returns a Driver.
func New(ctx context.Context, cfg *config.Config, tgt string) (*Driver, error) {
	cc := target.NewConnCache(cfg, tgt)
	conn, err := cc.Conn(ctx)
	if err != nil {
		cc.Close(ctx)
		return nil, err
	}
	return &Driver{
		cfg:  cfg,
		cc:   cc,
		conn: conn,
	}, nil
}

// Close closes the current connection to the target device.
func (d *Driver) Close(ctx context.Context) error {
	return d.cc.Close(ctx)
}

// Target returns a connection spec as [<user>@]host[:<port>].
func (d *Driver) Target() string {
	return d.cc.Target()
}

// InitBootID returns a boot ID string obtained on the first successful
// connection to the target device.
func (d *Driver) InitBootID() string {
	return d.cc.InitBootID()
}

// SSHConn returns ssh.Conn for the current connection.
// The return value may change after calling non-getter methods.
func (d *Driver) SSHConn() *ssh.Conn {
	return d.conn.SSHConn()
}

// Services returns a Services object that owns various services exposed to the
// target device.
// The return value may change after calling non-getter methods.
func (d *Driver) Services() *Services {
	return d.conn.Services()
}

// ReconnectIfNeeded ensures that the current connection is healthy, and
// otherwise it re-establishes a connection.
func (d *Driver) ReconnectIfNeeded(ctx context.Context) error {
	conn, err := d.cc.Conn(ctx)
	if err != nil {
		return err
	}
	d.conn = conn
	return nil
}

func (d *Driver) localClient() *runnerclient.JSONClient {
	var args []string
	if d.cfg.Proxy == config.ProxyEnv {
		// Proxy-related variables can be either uppercase or lowercase.
		// See https://golang.org/pkg/net/http/#ProxyFromEnvironment.
		for _, name := range []string{
			"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
			"http_proxy", "https_proxy", "no_proxy",
		} {
			if val := os.Getenv(name); val != "" {
				args = append(args, fmt.Sprintf("%s=%s", name, val))
			}
		}
	}
	args = append(args, d.cfg.LocalRunner)

	cmd := genericexec.CommandSSH(d.conn.SSHConn(), "env", args...)
	params := &protocol.RunnerInitParams{BundleGlob: d.cfg.LocalBundleGlob()}
	return runnerclient.NewJSONClient(cmd, params, 1)
}
