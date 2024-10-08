// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package driver implements communications with local/remote executables
// related to Tast.
package driver

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver/internal/drivercore"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver/internal/runnerclient"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver/internal/sshconfig"
	"go.chromium.org/tast/core/errors"

	"go.chromium.org/tast/core/ssh"

	"go.chromium.org/tast/core/internal/debugger"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/bundleclient"
	"go.chromium.org/tast/core/internal/minidriver/servo"
	"go.chromium.org/tast/core/internal/minidriver/target"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/genericexec"
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
	cfg              *config.Config
	cc               *target.ConnCache
	rawTarget        string
	role             string
	servoHostInfo    *servo.HostInfo
	remoteDevservers []string
}

// New establishes a new connection to the target device and returns a Driver.
func New(ctx context.Context, cfg *config.Config, rawTarget, role string, remoteDevservers []string) (*Driver, error) {
	servoHost := target.ServoHost(ctx, role, cfg.TestVars())
	dutLabConfig := cfg.DUTLabConfig()
	var dutTopology *api.Dut
	if dutLabConfig.GetChromeOSDUTLabConfig() != nil {
		dt, ok := dutLabConfig.GetChromeOSDUTLabConfig()[role]
		if ok {
			dutTopology = dt
		}
	}
	if dutTopology == nil && dutLabConfig.GetDevboardDUTLabConfig() != nil {
		dt, ok := dutLabConfig.GetDevboardDUTLabConfig()[role]
		if ok {
			dutTopology = dt
		}
	}
	servoHostInfo, err := servo.StartServo(ctx, servoHost,
		cfg.ProtoSSHConfig().GetKeyFile(), cfg.ProtoSSHConfig().GetKeyDir(),
		dutTopology)
	if err != nil {
		logging.Infof(ctx, "Failed to connect to servo host %s: %v",
			target.ServoHost(ctx, role, cfg.TestVars()), err)
	}
	// Use nil as connection cache if we should not connect to the target.
	if !config.ShouldConnect(cfg.Target()) {
		return &Driver{
			cfg:       cfg,
			rawTarget: rawTarget,
			role:      role,
		}, nil
	}

	resolvedTarget, resolvedProxyCommand := resolveSSHConfig(ctx, rawTarget)
	proxyCommand := cfg.ProtoSSHConfig().GetProxyCommand()
	if proxyCommand == "" {
		proxyCommand = resolvedProxyCommand
	}
	var debuggerPorts []int
	for _, dt := range []debugger.DebugTarget{debugger.LocalTestRunner, debugger.LocalBundle} {
		debugPort, ok := cfg.DebuggerPorts()[dt]
		if !ok || debugPort == 0 {
			continue
		}
		debuggerPorts = append(debuggerPorts, debugPort)
	}

	scfg := &target.ServiceConfig{
		TLWServer:              cfg.TLWServer(),
		UseEphemeralDevserver:  cfg.UseEphemeralDevserver(),
		Devservers:             cfg.Devservers(),
		TastDir:                cfg.TastDir(),
		ExtraAllowedBuckets:    cfg.ExtraAllowedBuckets(),
		DebuggerPorts:          debuggerPorts,
		DebuggerPortForwarding: cfg.DebuggerPortForwarding(),
	}
	tcfg := &target.Config{
		SSHConfig:     cfg.ProtoSSHConfig(),
		Retries:       cfg.Retries(),
		TastVars:      cfg.TestVars(),
		ServiceConfig: scfg,
	}
	cc, err := target.NewConnCache(ctx, tcfg, resolvedTarget, proxyCommand, role, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a new connection")
	}
	return &Driver{
		cfg:              cfg,
		cc:               cc,
		rawTarget:        rawTarget,
		role:             role,
		servoHostInfo:    servoHostInfo,
		remoteDevservers: remoteDevservers,
	}, nil
}

// Duplicate duplicate a driver.
func (d *Driver) Duplicate(ctx context.Context) (*Driver, error) {
	return New(ctx, d.cfg, d.rawTarget, d.role, d.remoteDevservers)
}

// Close closes the current connection to the target device.
func (d *Driver) Close(ctx context.Context) error {
	// Check we have the connection to close.
	if d.cc == nil {
		return nil
	}
	return d.cc.Close(ctx)
}

// ConnectionSpec returns a connection spec as [<user>@]host[:<port>].
func (d *Driver) ConnectionSpec() string {
	if d.cc == nil {
		return ""
	}
	return d.cc.ConnectionSpec()
}

// ProxyCommand returns the proxy command.
func (d *Driver) ProxyCommand() string {
	if d.cc == nil {
		return ""
	}
	return d.cc.ProxyCommand()
}

// InitBootID returns a boot ID string obtained on the first successful
// connection to the target device.
func (d *Driver) InitBootID() string {
	if d.cc == nil {
		return ""
	}
	return d.cc.InitBootID()
}

// SSHConn returns ssh.Conn for the current connection.
// The return value may change after calling non-getter methods.
func (d *Driver) SSHConn() *ssh.Conn {
	if d.cc == nil {
		return nil
	}
	return d.cc.Conn().SSHConn()
}

// Services returns a Services object that owns various services exposed to the
// target device.
// The return value may change after calling non-getter methods.
func (d *Driver) Services() *Services {
	if d.cc == nil {
		return nil
	}
	return d.cc.Conn().Services()
}

// ReconnectIfNeeded ensures that the current connection is healthy, and
// otherwise it re-establishes a connection.
func (d *Driver) ReconnectIfNeeded(ctx context.Context, rebootBeforeReconnect, quiet bool) error {
	if d.cc == nil {
		return nil
	}
	return d.cc.EnsureConn(ctx, rebootBeforeReconnect, quiet)
}

// DefaultTimeout returns the default timeout for connection operations.
func (d *Driver) DefaultTimeout() time.Duration {
	return d.cc.DefaultTimeout()
}

func (d *Driver) localRunnerClient() *runnerclient.Client {
	// We dont have access to the target.
	if !config.ShouldConnect(d.cfg.Target()) {
		return nil
	}
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
	bundlePath := filepath.Join(d.cfg.RemoteBundleDir(), bundle)
	cmd := genericexec.CommandExec(bundlePath)
	return bundleclient.New(cmd, d.cfg.MsgTimeout(), bundlePath)
}

func resolveSSHConfig(ctx context.Context, target string) (alternateTarget, proxyCommand string) {
	alternateTarget, proxyCommand, err := sshconfig.ResolveHost(target)
	if err != nil {
		logging.Infof(ctx, "Error in reading SSH configuaration files: %v", err)
		return target, ""
	}
	if alternateTarget != target {
		logging.Infof(ctx, "Using target %v instead of %v to connect according to SSH configuration files",
			alternateTarget, target)
	}
	return alternateTarget, proxyCommand
}

// ConnCacheForTesting returns target.ConnCache the driver owns for testing.
func (d *Driver) ConnCacheForTesting() *target.ConnCache {
	return d.cc
}

// RemoteDevservers returns the remote devservers associated with the driver.
func (d *Driver) RemoteDevservers() []string {
	return d.remoteDevservers
}

// CollectServoLogs collects the sysinfo, considering diff from the given initial
// sysinfo state.
func (d *Driver) CollectServoLogs(ctx context.Context) (retErr error) {
	servoLogDir := filepath.Join(d.cfg.ResDir(), "servo_log")
	if err := servo.CleanUpAndCollectLogs(ctx, d.servoHostInfo, d.cfg.ProtoSSHConfig().GetKeyFile(),
		d.cfg.ProtoSSHConfig().GetKeyDir(), servoLogDir); err != nil {
		logging.Infof(ctx, "Failed to get to servo log: %v", err)
	}
	return nil
}
