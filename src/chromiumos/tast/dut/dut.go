// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/testingutil"
	"chromiumos/tast/ssh"
)

const (
	pingTimeout    = time.Second
	pingRetryDelay = time.Second

	connectTimeout      = 10 * time.Second
	reconnectRetryDelay = time.Second
)

// Suffix names for forward compatibility.
const (
	// CompanionSuffixPcap is a companion suffix for the pcap.
	CompanionSuffixPcap = "-pcap"
	// CompanionSuffixRouter is a companion suffix for the router.
	CompanionSuffixRouter = "-router"
	// CompanionSuffixTablet is a companion suffix for the tablet.
	CompanionSuffixTablet = "-tablet"
)

// DUT represents a "Device Under Test" against which remote tests are run.
type DUT struct {
	sopt         ssh.Options
	hst          *ssh.Conn
	beforeReboot func(context.Context, *DUT) error
}

// New returns a new DUT usable for communication with target
// (of the form "[<user>@]host[:<port>]") using the SSH key at keyFile or
// keys located in keyDir.
// The DUT does not start out in a connected state; Connect must be called.
func New(target, keyFile, keyDir string, beforeReboot func(context.Context, *DUT) error) (*DUT, error) {
	d := DUT{beforeReboot: beforeReboot}
	if err := ssh.ParseTarget(target, &d.sopt); err != nil {
		return nil, err
	}
	d.sopt.ConnectTimeout = connectTimeout
	d.sopt.KeyFile = keyFile
	d.sopt.KeyDir = keyDir

	return &d, nil
}

// Conn returns the connection to the DUT, or nil if there is no connection.
// The ownership of the connection is managed by *DUT, so don't call Close for
// the connection. To disconnect, call DUT.Disconnect.
// Storing the returned value into a variable is not recommended, because
// after reconnection (e.g. by Reboot), the instance previous Conn returned
// is stale. Always call Conn to get the present connection.
//
// Examples:
//  linuxssh.GetFile(ctx, d.Conn(), src, dst, linuxssh.PreserveSymlinks)
//  d.Conn().CommandContext("uptime")
func (d *DUT) Conn() *ssh.Conn {
	return d.hst
}

// Close releases the DUT's resources.
func (d *DUT) Close(ctx context.Context) error {
	return d.Disconnect(ctx)
}

// Connected returns true if a usable connection to the DUT is held.
func (d *DUT) Connected(ctx context.Context) bool {
	if d.hst == nil {
		return false
	}
	if err := d.hst.Ping(ctx, pingTimeout); err != nil {
		return false
	}
	return true
}

// Connect establishes a connection to the DUT. If a connection already
// exists, it is closed first.
func (d *DUT) Connect(ctx context.Context) error {
	d.Disconnect(ctx)

	var err error
	d.hst, err = ssh.New(ctx, &d.sopt)
	if err != nil {
		return err
	}
	logging.Info(ctx, "Opened DUT SSH connection to ", d.sopt.Hostname)
	return nil
}

// Disconnect closes the current connection to the DUT. It is a no-op if
// no connection is currently established.
func (d *DUT) Disconnect(ctx context.Context) error {
	if d.hst == nil {
		return nil
	}
	defer func() { d.hst = nil }()
	logging.Info(ctx, "Closing DUT SSH connection to ", d.sopt.Hostname)
	return d.hst.Close(ctx)
}

// GetFile copies a file or directory from the DUT to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
//
// DEPRECATED: use linuxssh.GetFile(ctx, d.Conn(), src, dst, linuxssh.PreserveSymlinks)
func (d *DUT) GetFile(ctx context.Context, src, dst string) error {
	return linuxssh.GetFile(ctx, d.hst, src, dst, linuxssh.PreserveSymlinks)
}

// WaitUnreachable waits for the DUT to become unreachable.
func (d *DUT) WaitUnreachable(ctx context.Context) error {
	if d.hst == nil {
		deadline, ok := ctx.Deadline()
		if ok && deadline.Before(time.Now().Add(d.sopt.ConnectTimeout)) {
			// There isn't enough time to connect
			return errors.Errorf("context timeout too short, need at least %s, got %s", d.sopt.ConnectTimeout, deadline.Sub(time.Now()))
		}
		if err := d.Connect(ctx); err != nil {
			// Return the context's error instead of the one returned by Connect:
			// we should return an error if the context's deadline expired,
			// while returning nil if only Connect returned an error.
			return ctx.Err()
		}
	}

	logging.Infof(ctx, "Waiting for %s to be unreachable.", d.sopt.Hostname)
	for {
		deadline, ok := ctx.Deadline()
		if ok && deadline.Before(time.Now().Add(pingTimeout)) {
			// There isn't enough time to ping
			return errors.Errorf("context timeout too short, need at least %s, got %s", pingTimeout, deadline.Sub(time.Now()))
		}
		if err := d.hst.Ping(ctx, pingTimeout); err != nil {
			// Return the context's error instead of the one returned by Ping:
			// we should return an error if the context's deadline expired,
			// while returning nil if only Ping returned an error.
			return ctx.Err()
		}

		select {
		case <-time.After(pingRetryDelay):
			break
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// WaitConnect connects to the DUT, waiting for it to become reachable.
// If a connection already exists, it is closed first.
func (d *DUT) WaitConnect(ctx context.Context) error {
	logging.Infof(ctx, "Waiting for %s to connect.", d.sopt.Hostname)
	for {
		err := d.Connect(ctx)
		if err == nil {
			return nil
		}

		select {
		case <-time.After(reconnectRetryDelay):
			break
		case <-ctx.Done():
			if err.Error() == ctx.Err().Error() {
				return err
			}
			return fmt.Errorf("%v (%v)", ctx.Err(), err)
		}
	}
}

// Reboot reboots the DUT.
func (d *DUT) Reboot(ctx context.Context) error {
	if d.beforeReboot != nil {
		if err := d.beforeReboot(ctx, d); err != nil {
			return errors.Wrap(err, "failed while running pre-reboot function")
		}
	}
	readBootID := func(ctx context.Context) (string, error) {
		out, err := d.Conn().CommandContext(ctx, "cat", "/proc/sys/kernel/random/boot_id").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}

	initID, err := readBootID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read initial boot_id")
	}

	// Run the reboot command with a short timeout. This command can block for long time
	// if the network interface of the DUT goes down before the SSH command finishes.
	rebootCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	d.Conn().CommandContext(rebootCtx, "reboot").Run() // ignore the error

	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		// Set a short timeout to the iteration in case of any SSH operations
		// blocking for long time. For example, the network interface of the DUT
		// might go down in the middle of readBootID and it might block for
		// long time.
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := d.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT")
		}
		curID, err := readBootID(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to read boot_id")
		}
		if curID == initID {
			return errors.New("boot_id did not change")
		}
		return nil
	}, &testingutil.PollOptions{Timeout: 3 * time.Minute}); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to reboot")
	}
	return nil
}

// KeyFile returns the path to the SSH private key used to connect to the DUT.
// This is provided for tests that may need to establish SSH connections to additional hosts
// (e.g. a host running a servod instance).
func (d *DUT) KeyFile() string { return d.sopt.KeyFile }

// KeyDir returns the path to the directory containing SSH private keys used to connect to the DUT.
// This is provided for tests that may need to establish SSH connections to additional hosts
// (e.g. a host running a servod instance).
func (d *DUT) KeyDir() string { return d.sopt.KeyDir }

// HostName returns the a string representing the "<dut_hostname>:<ssh_port>" used to connect to the DUT.
// This is provided for tests that may need to establish direct SSH connections to hosts.
// (e.g. syzkaller connecting to a host).
func (d *DUT) HostName() string { return d.sopt.Hostname }

// ErrCompanionHostname is the error of deriving default companion device hostname from dut's hostname.
// e.g. when DUT is connected with IP address.
var ErrCompanionHostname = errors.New("cannot derive default companion device hostname")

// CompanionDeviceHostname derives the hostname of companion device from test target
// with the convention in Autotest.
// (see server/cros/dnsname_mangler.py in Autotest)
func (d *DUT) CompanionDeviceHostname(suffix string) (string, error) {
	dutHost := d.sopt.Hostname
	// Try split out port part.
	if host, _, err := net.SplitHostPort(dutHost); err == nil {
		dutHost = host
	}

	if ip := net.ParseIP(dutHost); ip != nil {
		// We don't mangle IP address. Return error.
		return "", ErrCompanionHostname
	}

	// Companion device hostname convention: append suffix after the first sub-domain string.
	hostname := strings.SplitN(dutHost, ".", 2)
	hostname[0] = hostname[0] + suffix
	return strings.Join(hostname, "."), nil
}

// connectCompanionDevice connects to a companion device in test environment. e.g. WiFi AP.
// It reuses SSH key from DUT for establishing SSH connection to a companion device.
// Note that it uses the derived hostname (from the DUT's hostname) with default port to
// establish a connection. If the router/pcap runs in a non-default port, router/pcap
// target should be specified in test variables.
func (d *DUT) connectCompanionDevice(ctx context.Context, suffix string) (*ssh.Conn, error) {
	var sopt ssh.Options
	hostname, err := d.CompanionDeviceHostname(suffix)
	if err != nil {
		return nil, err
	}
	// Let ParseTarget derive default user and port for us.
	if err := ssh.ParseTarget(hostname, &sopt); err != nil {
		return nil, ErrCompanionHostname
	}
	sopt.ConnectTimeout = connectTimeout
	// Companion devices use the same key as DUT.
	sopt.KeyFile = d.sopt.KeyFile
	sopt.KeyDir = d.sopt.KeyDir

	return ssh.New(ctx, &sopt)
}

// DefaultWifiRouterHost connects to the default WiFi router and returns SSH object.
// DEPRECATED: Connect using CompanionDeviceHostname() instead.
func (d *DUT) DefaultWifiRouterHost(ctx context.Context) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, CompanionSuffixRouter)
}

// DefaultWifiPcapHost connects to the default WiFi pcap router and returns SSH object.
// DEPRECATED: Connect using CompanionDeviceHostname() instead.
func (d *DUT) DefaultWifiPcapHost(ctx context.Context) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, CompanionSuffixPcap)
}

// WifiPeerHost connects to the WiFi peer (specified by index) and returns SSH object.
func (d *DUT) WifiPeerHost(ctx context.Context, index int) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, fmt.Sprintf("-wifipeer%d", index))
}

// DefaultCameraboxChart connects to paired chart tablet in camerabox setup and returns SSH object.
func (d *DUT) DefaultCameraboxChart(ctx context.Context) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, CompanionSuffixTablet)
}

// NewSecondaryDevice creates a DUT for a secondary target, sharing the same SSH info.
// TODO(crbug/1129234): Remove this when full secondary DUT support is added.
func (d *DUT) NewSecondaryDevice(target string) (*DUT, error) {
	d2 := DUT{beforeReboot: d.beforeReboot}
	if err := ssh.ParseTarget(target, &d2.sopt); err != nil {
		return nil, err
	}
	d2.sopt.ConnectTimeout = connectTimeout
	d2.sopt.KeyFile = d.sopt.KeyFile
	d2.sopt.KeyDir = d.sopt.KeyDir

	return &d2, nil
}
