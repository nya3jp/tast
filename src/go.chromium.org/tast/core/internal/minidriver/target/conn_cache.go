// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/linuxssh"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/testingutil"
)

// ConnCache manages a cached connection to the target.
//
// ConnCache is not goroutine-safe.
type ConnCache struct {
	cfg          *Config
	target       string
	proxyCommand string
	initBootID   string
	helper       *dutHelper
	conn         *Conn // always non-nil
}

// NewConnCache establishes a new connection to target and return a ConnCache.
// Call Close when it is no longer needed.
func NewConnCache(ctx context.Context, cfg *Config, target, proxyCommand, role string) (cc *ConnCache, retErr error) {
	helper := newDUTHelper(ctx, cfg.SSHConfig, cfg.TastVars, role)
	conn, err := newConn(ctx, cfg, target, proxyCommand, helper.dutServer)
	if err != nil {
		if err := ensureDUTConnection(ctx, cfg.SSHConfig.ConnectionSpec); err != nil {
			logging.Infof(ctx, "Failed to connect to DUT %s", err)
		}
		conn, err = newConn(ctx, cfg, target, proxyCommand, helper.dutServer)
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		if retErr != nil {
			conn.close(ctx)
		}
	}()

	bootID, err := linuxssh.ReadBootID(ctx, conn.SSHConn())
	if err != nil {
		return nil, errors.Wrap(err, "failed to read boot id")
	}

	return &ConnCache{
		cfg:          cfg,
		target:       target,
		proxyCommand: proxyCommand,
		initBootID:   bootID,
		conn:         conn,
		helper:       helper,
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

// EnsureConn ensures a cached connection to the target is available and
// healthy, possibly creating a new connection.
//
// EnsureConn has slight performance hit because it checks the connection health
// by sending SSH ping. Unless you expect connection drops (e.g. after running
// test bundles), use Conn without calling this method.
//
// Be aware that calling EnsureConn may invalidate a connection previously
// returned from Conn.
func (cc *ConnCache) EnsureConn(ctx context.Context, rebootBeforeReconnect bool) error {
	err := cc.conn.Healthy(ctx)
	if err == nil {
		return nil
	}
	logging.Debugf(ctx, "Target connection is unhealthy: %v; reconnecting", err)

	newConnection, err := newConn(ctx, cc.cfg, cc.target, cc.proxyCommand, cc.helper.dutServer)
	if err != nil {
		// b/205333029: Move the code for rebooting to somewhere else when we support servod for multiple DUT.
		if cc.helper != nil && rebootBeforeReconnect {
			// If we have a way, reboot the DUT.
			logging.Info(ctx, "Reboot target before reconnecting")
			if rebootErr := cc.helper.HardReboot(ctx); rebootErr != nil {
				logging.Infof(ctx, "Fail to reboot target: %v", rebootErr)
			}
			shortCtx, cancel := context.WithTimeout(ctx, time.Minute*3)
			defer cancel()
			if err := testingutil.Poll(shortCtx, func(ctx context.Context) error {
				newConnection, err = newConn(ctx, cc.cfg, cc.target, cc.proxyCommand, cc.helper.dutServer)
				return err
			}, &testingutil.PollOptions{Timeout: cc.DefaultTimeout()}); err != nil {
				logging.Infof(ctx, "Fail to reconnect after reboot target: %v", err)
			}
		}
		if err != nil {
			return err
		}
	}

	cc.conn.close(ctx)
	cc.conn = newConnection

	return nil
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

// ProxyCommand returns the proxy command.
func (cc *ConnCache) ProxyCommand() string {
	return cc.proxyCommand
}

// DefaultTimeout returns the default timeout for connection operations.
func (cc *ConnCache) DefaultTimeout() time.Duration {
	if cc.helper != nil {
		return time.Minute * 5
	}
	return time.Minute
}
