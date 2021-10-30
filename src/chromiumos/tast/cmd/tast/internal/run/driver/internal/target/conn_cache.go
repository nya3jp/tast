// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
)

// ConnCache manages a cached connection to the target.
//
// ConnCache is not goroutine-safe.
type ConnCache struct {
	cfg        *config.Config
	target     string
	initBootID string
	dutAction  DUTAction

	conn *Conn // always non-nil
}

// DUTAction provides an interface to provide action on a DUT.
type DUTAction interface {
	// Reboot a DUt.
	HardReboot(ctx context.Context) error
}

// NewConnCache establishes a new connection to target and return a ConnCache.
// Call Close when it is no longer needed.
func NewConnCache(ctx context.Context, cfg *config.Config, target string, dutAction DUTAction) (cc *ConnCache, retErr error) {
	conn, err := newConn(ctx, cfg, target)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			conn.close(ctx)
		}
	}()

	bootID, err := linuxssh.ReadBootID(ctx, conn.SSHConn())
	if err != nil {
		return nil, err
	}

	return &ConnCache{
		cfg:        cfg,
		target:     target,
		initBootID: bootID,
		conn:       conn,
		dutAction:  dutAction,
	}, nil
}

// Close closes a cached connection.
func (cc *ConnCache) Close(ctx context.Context) error {
	err := cc.conn.close(ctx)
	cc.conn = nil
	return err
}

// Conn returns a cached connection to the target.
//
// A connection returned from this function is owned by ConnCache. Do not call
// its Close.
func (cc *ConnCache) Conn() *Conn {
	return cc.conn
}

// EnsureConn returns a cached connection to the target if it is available and
// healthy. Otherwise EnsureConn creates a new connection.
//
// Even when ConnCache has a cached connection, getting it with EnsureConn has
// slight performance hit because it checks the connection health by sending SSH
// ping. Unless you expect connection drops (e.g. after running test bundles),
// use Conn instead to get a cached connection without checking connection
// health.
//
// Be aware that calling EnsureConn may invalidate a connection previously
// returned from Conn/EnsureConn.
//
// A connection returned from this function is owned by ConnCache. Do not call
// its Close.
func (cc *ConnCache) EnsureConn(ctx context.Context) (conn *Conn, retErr error) {
	err := cc.conn.Healthy(ctx)
	if err == nil {
		return cc.conn, nil
	}
	logging.Infof(ctx, "Target connection is unhealthy: %v; reconnecting", err)

	// use a context with short timeout to try reconnection and close.
	shortCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	newConnection, err := newConn(shortCtx, cc.cfg, cc.target)
	if err != nil {
		if cc.dutAction != nil {
			// If we have control of the DUt, reboot the DUT.
			logging.Info(ctx, "Reboot target before reconnecting")
			if err := cc.dutAction.HardReboot(ctx); err != nil {
				logging.Infof(ctx, "Fail to reboot target: %v", err)
			}
			// Sleep for 3 minutes to make sure everything is up.
			time.Sleep(time.Minute * 3)
			newConnection, err = newConn(shortCtx, cc.cfg, cc.target)
		}
		if err != nil {
			return nil, err
		}
	}

	cc.conn.close(shortCtx)
	cc.conn = newConnection

	return newConnection, nil
}

// InitBootID returns a boot ID string obtained on the first successful
// connection to the target. An empty string is returned if it is unavailable.
func (cc *ConnCache) InitBootID() string {
	return cc.initBootID
}

// ConnectionSpec returns a connection spec as [<user>@]host[:<port>].
func (cc *ConnCache) ConnectionSpec() string {
	return cc.target
}
