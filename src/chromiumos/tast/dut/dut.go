// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/host"
	"chromiumos/tast/testing"
)

const (
	dutKey key = iota // key used for attaching a DUT to a context.Context
)

const (
	pingTimeout    = time.Second
	pingRetryDelay = time.Second

	connectTimeout      = 10 * time.Second
	reconnectRetryDelay = time.Second
)

type key int // unexported context.Context key type to avoid collisions with other packages

// DUT represents a "Device Under Test" against which remote tests are run.
type DUT struct {
	sopt host.SSHOptions
	hst  *host.SSH
}

// NewContext returns a new context that carries value d.
func NewContext(ctx context.Context, d *DUT) context.Context {
	return context.WithValue(ctx, dutKey, d)
}

// FromContext returns the DUT value stored in ctx, if any.
func FromContext(ctx context.Context) (d *DUT, ok bool) {
	d, ok = ctx.Value(dutKey).(*DUT)
	return d, ok
}

// New returns a new DUT usable for communication with target
// (of the form "[<user>@]host[:<port>]") using the SSH key at keyFile or
// keys located in keyDir.
// The DUT does not start out in a connected state; Connect must be called.
func New(target, keyFile, keyDir string) (*DUT, error) {
	d := DUT{}
	if err := host.ParseSSHTarget(target, &d.sopt); err != nil {
		return nil, err
	}
	d.sopt.ConnectTimeout = connectTimeout
	d.sopt.KeyFile = keyFile
	d.sopt.KeyDir = keyDir

	return &d, nil
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
	d.hst, err = host.NewSSH(ctx, &d.sopt)
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

// Run runs cmd synchronously on the DUT and returns combined stdout and stderr.
//
// This method is deprecated and will be removed soon. Use Command instead.
func (d *DUT) Run(ctx context.Context, cmd string) ([]byte, error) {
	return d.Command("sh", "-c", cmd).CombinedOutput(ctx)
}

// Command returns the Cmd struct to execute the named program with the given arguments.
//
// See https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/host#Command
func (d *DUT) Command(name string, args ...string) *host.Cmd {
	// It is fine even if d.hst is nil; subsequent method calls will just fail.
	return d.hst.Command(name, args...)
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

	if err := testing.Poll(ctx, func(ctx context.Context) error {
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
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
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
