// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bundleclient provides bundle services client implementation.
package bundleclient

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/ssh"
)

// Client is a bundle client.
type Client struct {
	hst *ssh.Conn
	cl  *rpc.SSHClient
}

// Close closes the underlying connections.
func (c *Client) Close(ctx context.Context) error {
	err := c.cl.Close()
	err2 := c.hst.Close(ctx)
	if err != nil {
		return err
	}
	return err2
}

// TestService returns test service client.
func (c *Client) TestService() protocol.TestServiceClient {
	return protocol.NewTestServiceClient(c.cl.Conn())
}

// SSHConn returns the underlying SSH connection.
func (c *Client) SSHConn() *ssh.Conn {
	return c.hst
}

// New creates a client connecting to a bundle in a different machine.
// Returned client is suitable for framework to use.
// target specifies the target bundle.
// name represents the bundle basename, like "cros".
func New(ctx context.Context, target *protocol.TargetDevice, name string, hr *protocol.HandshakeRequest) (_ *Client, retErr error) {
	var opts ssh.Options
	scfg := target.GetDutConfig().GetSshConfig()
	if err := ssh.ParseTarget(scfg.GetConnectionSpec(), &opts); err != nil {
		return nil, err
	}
	opts.ConnectTimeout = 10 * time.Second
	opts.KeyFile = scfg.GetKeyFile()
	opts.KeyDir = scfg.GetKeyDir()

	hst, err := ssh.New(ctx, &opts)
	if err != nil {
		return nil, errors.Wrapf(err, "connecting to %s", scfg.GetConnectionSpec())
	}
	defer func() {
		if retErr != nil {
			hst.Close(ctx)
		}
	}()

	path := filepath.Join(target.GetBundleDir(), name)

	cl, err := rpc.DialSSH(ctx, hst, path, hr, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			cl.Close()
		}
	}()

	return &Client{hst, cl}, nil
}
