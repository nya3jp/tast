// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
)

// ConnCache manages a cached connection to the target.
//
// ConnCache is not goroutine-safe.
type ConnCache struct {
	cfg    *config.Config
	target string

	conn       *Conn
	initBootID string
}

// NewConnCache creates a new cache. Call Close when it is no longer needed.
func NewConnCache(cfg *config.Config, target string) *ConnCache {
	return &ConnCache{cfg: cfg, target: target}
}

// Close closes a cached connection if any.
func (cc *ConnCache) Close(ctx context.Context) error {
	if cc.conn == nil {
		return nil
	}
	err := cc.conn.close(ctx)
	cc.conn = nil
	return err
}

// Conn returns a cached connection to the target if it is available and
// healthy. Otherwise Conn creates a new connection.
//
// Even when ConnCache has a cached connection, getting it with Conn has slight
// performance hit because it checks the connection health by sending SSH ping.
// Unless you expect connection drops (e.g. after running test bundles), try to
// keep using a returned connection. As a corollary, there is nothing wrong with
// functions taking both ConnCache and ssh.Conn as arguments.
//
// Be aware that calling Conn may invalidate a connection previously returned
// from the function.
//
// A connection returned from this function is owned by ConnCache. Do not call
// its Close.
func (cc *ConnCache) Conn(ctx context.Context) (conn *Conn, retErr error) {
	// If we already have a connection, reuse it if it's still open.
	if cc.conn != nil {
		err := cc.conn.Healthy(ctx)
		if err == nil {
			return cc.conn, nil
		}
		logging.Infof(ctx, "Target connection is unhealthy: %v; reconnecting", err)
		cc.Close(ctx)
	}

	conn, err := newConn(ctx, cc.cfg, cc.target)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			conn.close(ctx)
		}
	}()

	// If this is the first time to connect to the target, save boot ID for
	// comparison later.
	var bootID string
	if cc.initBootID == "" {
		bootID, err = linuxssh.ReadBootID(ctx, conn.SSHConn())
		if err != nil {
			return nil, err
		}
	}

	cc.conn = conn
	if cc.initBootID == "" {
		cc.initBootID = bootID
	}

	return cc.conn, nil
}

// InitBootID returns a boot ID string obtained on the first successful
// connection to the target. An empty string is returned if it is unavailable.
func (cc *ConnCache) InitBootID() string {
	return cc.initBootID
}

// Target returns a connection spec as [<user>@]host[:<port>].
func (cc *ConnCache) Target() string {
	return cc.target
}
