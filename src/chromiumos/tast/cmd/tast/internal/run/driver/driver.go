// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package driver implements communications with local/remote executables
// related to Tast.
package driver

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/drivercore"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/sshconfig"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/bundleclient"
	"chromiumos/tast/internal/minidriver/target"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/genericexec"
	"chromiumos/tast/ssh"
)

const (
	// SSHPingTimeout is the timeout for checking if SSH connection to DUT is open.
	SSHPingTimeout = target.SSHPingTimeout
)

// Services owns services exposed to a target device by SSH port forwarding.
type Services = target.Services

// BundleEntity is a pair of a ResolvedEntity and its bundle name.
type BundleEntity = drivercore.BundleEntity

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
}

// New establishes a new connection to the target device and returns a Driver.
func New(ctx context.Context, cfg *config.Config, rawTarget, role string) (*Driver, error) {
	resolvedTarget := resolveSSHConfig(ctx, rawTarget)

	var debuggerPorts []int
	for _, dt := range []debugger.DebugTarget{debugger.LocalTestRunner, debugger.LocalBundle} {
		debugPort, ok := cfg.DebuggerPorts()[dt]
		if !ok || debugPort == 0 {
			continue
		}
		debuggerPorts = append(debuggerPorts, debugPort)
	}

	scfg := &target.ServiceConfig{
		TLWServer:             cfg.TLWServer(),
		UseEphemeralDevserver: cfg.UseEphemeralDevserver(),
		Devservers:            cfg.Devservers(),
		TastDir:               cfg.TastDir(),
		ExtraAllowedBuckets:   cfg.ExtraAllowedBuckets(),
		DebuggerPorts:         debuggerPorts,
	}
	tcfg := &target.Config{
		SSHConfig:     cfg.ProtoSSHConfig(),
		Retries:       cfg.Retries(),
		TastVars:      cfg.TestVars(),
		ServiceConfig: scfg,
	}
	cc, err := target.NewConnCache(ctx, tcfg, resolvedTarget, role)
	if err != nil {
		return nil, err
	}
	return &Driver{
		cfg: cfg,
		cc:  cc,
	}, nil
}

// Close closes the current connection to the target device.
func (d *Driver) Close(ctx context.Context) error {
	return d.cc.Close(ctx)
}

// ConnectionSpec returns a connection spec as [<user>@]host[:<port>].
func (d *Driver) ConnectionSpec() string {
	return d.cc.ConnectionSpec()
}

// InitBootID returns a boot ID string obtained on the first successful
// connection to the target device.
func (d *Driver) InitBootID() string {
	return d.cc.InitBootID()
}

// SSHConn returns ssh.Conn for the current connection.
// The return value may change after calling non-getter methods.
func (d *Driver) SSHConn() *ssh.Conn {
	return d.cc.Conn().SSHConn()
}

// Services returns a Services object that owns various services exposed to the
// target device.
// The return value may change after calling non-getter methods.
func (d *Driver) Services() *Services {
	return d.cc.Conn().Services()
}

// ReconnectIfNeeded ensures that the current connection is healthy, and
// otherwise it re-establishes a connection.
func (d *Driver) ReconnectIfNeeded(ctx context.Context) error {
	return d.cc.EnsureConn(ctx)
}

// DefaultTimeout returns the default timeout for connection operations.
func (d *Driver) DefaultTimeout() time.Duration {
	return d.cc.DefaultTimeout()
}

func (d *Driver) localRunnerClient() *runnerclient.Client {
	cmd := bundleclient.LocalCommand(d.cfg.LocalRunner(), d.cfg.Proxy() == config.ProxyEnv, d.cc)

	params := &protocol.RunnerInitParams{BundleGlob: d.cfg.LocalBundleGlob()}
	return runnerclient.New(cmd, params, d.cfg.MsgTimeout(), 1)
}

func (d *Driver) remoteRunnerClient() *runnerclient.Client {
	cmd := genericexec.CommandExec(d.cfg.RemoteRunner())
	params := &protocol.RunnerInitParams{BundleGlob: d.cfg.RemoteBundleGlob()}
	return runnerclient.New(cmd, params, d.cfg.MsgTimeout(), 0)
}

func (d *Driver) remoteBundleClient(bundle string) *bundleclient.Client {
	cmd := genericexec.CommandExec(filepath.Join(d.cfg.RemoteBundleDir(), bundle))
	return bundleclient.New(cmd)
}

func resolveSSHConfig(ctx context.Context, target string) string {
	alternateTarget, err := sshconfig.ResolveHost(target)
	if err != nil {
		logging.Infof(ctx, "Error in reading SSH configuaration files: %v", err)
		return target
	}
	if alternateTarget != target {
		logging.Infof(ctx, "Using target %v instead of %v to connect according to SSH configuration files",
			alternateTarget, target)
	}
	return alternateTarget
}

// ConnCacheForTesting returns target.ConnCache the driver owns for testing.
func (d *Driver) ConnCacheForTesting() *target.ConnCache {
	return d.cc
}
