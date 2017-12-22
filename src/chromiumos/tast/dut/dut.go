// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"
	"errors"
	"fmt"
	"time"

	"chromiumos/tast/host"
)

const (
	dutKey key = iota // key used for attaching a DUT to a context.Context

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
// (of the form "[<user>@]host[:<port>]") using the SSH key at keyPath.
// The DUT does not start out in a connected state; Connect must be called.
func New(target, keyPath string) (*DUT, error) {
	d := DUT{}
	if err := host.ParseSSHTarget(target, &d.sopt); err != nil {
		return nil, err
	}
	d.sopt.ConnectTimeout = connectTimeout
	d.sopt.KeyPath = keyPath

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
func (d *DUT) Run(ctx context.Context, cmd string) ([]byte, error) {
	if d.hst == nil {
		return nil, errors.New("not connected")
	}
	return d.hst.Run(ctx, cmd)
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
			return fmt.Errorf("%v (%v)", ctx.Err(), err)
		}
	}
}
