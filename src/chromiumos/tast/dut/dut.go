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
	"chromiumos/tast/internal/testingutil"
	"chromiumos/tast/ssh"
)

const (
	pingTimeout    = time.Second
	pingRetryDelay = time.Second

	connectTimeout      = 10 * time.Second
	reconnectRetryDelay = time.Second
)

// DUT represents a "Device Under Test" against which remote tests are run.
type DUT struct {
	sopt ssh.Options
	hst  *ssh.Conn
}

// New returns a new DUT usable for communication with target
// (of the form "[<user>@]host[:<port>]") using the SSH key at keyFile or
// keys located in keyDir.
// The DUT does not start out in a connected state; Connect must be called.
func New(target, keyFile, keyDir string) (*DUT, error) {
	d := DUT{}
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
//  linuxssh.GetFile(ctx, d.Conn(), src, dst)
//  d.Conn().Command("uptime")
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
	return err
}

// Disconnect closes the current connection to the DUT. It is a no-op if
// no connection is currently established.
func (d *DUT) Disconnect(ctx context.Context) error {
	if d.hst == nil {
		return nil
	}
	defer func() { d.hst = nil }()
	return d.hst.Close(ctx)
}

// Command returns the Cmd struct to execute the named program with the given arguments.
//
// See https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/host#Command
//
// DEPRECATED: use d.Conn().Command
func (d *DUT) Command(name string, args ...string) *ssh.Cmd {
	// It is fine even if d.hst is nil; subsequent method calls will just fail.
	return d.hst.Command(name, args...)
}

// GetFile copies a file or directory from the DUT to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
//
// DEPRECATED: use linuxssh.GetFile(ctx, d.Conn(), src, dst)
func (d *DUT) GetFile(ctx context.Context, src, dst string) error {
	return linuxssh.GetFile(ctx, d.hst, src, dst)
}

// WaitUnreachable waits for the DUT to become unreachable.
// It requires that a connection is already established to the DUT.
func (d *DUT) WaitUnreachable(ctx context.Context) error {
	if d.hst == nil {
		return errors.New("not connected")
	}

	for {
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
	readBootID := func(ctx context.Context) (string, error) {
		out, err := d.Command("cat", "/proc/sys/kernel/random/boot_id").Output(ctx)
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
	d.Command("reboot").Run(rebootCtx) // ignore the error

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

// companionDeviceHostname derives the hostname of companion device from test target
// with the convention in Autotest.
// (see server/cros/dnsname_mangler.py in Autotest)
func companionDeviceHostname(dutHost, suffix string) (string, error) {
	// Try split out port part.
	if host, _, err := net.SplitHostPort(dutHost); err == nil {
		dutHost = host
	}

	if ip := net.ParseIP(dutHost); ip != nil {
		// We don't mangle IP address. Return error.
		return "", ErrCompanionHostname
	}

	// Companion device hostname convention: append suffix after the first sub-domain string.
	d := strings.SplitN(dutHost, ".", 2)
	d[0] = d[0] + suffix
	return strings.Join(d, "."), nil
}

// connectCompanionDevice connects to a companion device in test environment. e.g. WiFi AP.
// It reuses SSH key from DUT for establishing SSH connection to a companion device.
// Note that it uses the derived hostname (from the DUT's hostname) with default port to
// establish a connection. If the router/pcap runs in a non-default port, router/pcap
// target should be specified in test variables.
func (d *DUT) connectCompanionDevice(ctx context.Context, suffix string) (*ssh.Conn, error) {
	var sopt ssh.Options
	hostname, err := companionDeviceHostname(d.sopt.Hostname, suffix)
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
func (d *DUT) DefaultWifiRouterHost(ctx context.Context) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, "-router")
}

// DefaultWifiPcapHost connects to the default WiFi pcap router and returns SSH object.
func (d *DUT) DefaultWifiPcapHost(ctx context.Context) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, "-pcap")
}

// WifiPeerHost connects to the WiFi peer (specified by index) and returns SSH object.
func (d *DUT) WifiPeerHost(ctx context.Context, index int) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, fmt.Sprintf("-wifipeer%d", index))
}

// DefaultCameraboxChart connects to paired chart tablet in camerabox setup and returns SSH object.
func (d *DUT) DefaultCameraboxChart(ctx context.Context) (*ssh.Conn, error) {
	return d.connectCompanionDevice(ctx, "-tablet")
}
