// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package driver implements communications with local/remote executables
// related to Tast.
package driver

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/ssh"
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
