// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dbusutil provides additional functionality on top of the godbus/dbus package.
package dbusutil

import (
	"context"
	"fmt"

	"github.com/godbus/dbus"
)

const (
	signalChanSize = 10 // buffer size of channels holding signals
)

// SignalWatcher watches for and returns D-Bus signals matching a given pattern.
type SignalWatcher struct {
	// Signals passes signals matched by the MatchSpec passed to NewSignalWatcher.
	// This channel is buffered but must be serviced regularly; otherwise incoming
	// signals may be dropped.
	Signals chan *dbus.Signal

	conn    *dbus.Conn
	spec    MatchSpec
	allSigs chan *dbus.Signal // all signals received by the client
}

// NewSignalWatcher returns a new SignalWatcher that will return signals on conn matched by spec.
func NewSignalWatcher(ctx context.Context, conn *dbus.Conn, spec MatchSpec) (*SignalWatcher, error) {
	if err := conn.BusObject().Call(BusInterface+".AddMatch", 0, spec.String()).Err; err != nil {
		return nil, err
	}

	sw := &SignalWatcher{
		Signals: make(chan *dbus.Signal, signalChanSize),
		conn:    conn,
		spec:    spec,
		allSigs: make(chan *dbus.Signal, signalChanSize),
	}

	go func() {
		for sig := range sw.allSigs {
			if sw.spec.MatchesSignal(sig) {
				sw.Signals <- sig
			}
		}
	}()
	conn.Signal(sw.allSigs)

	return sw, nil
}

// NewSignalWatcherForSystemBus is a convenience function that calls NewSignalWatcher with
// a shared connection to the system bus.
func NewSignalWatcherForSystemBus(ctx context.Context, spec MatchSpec) (*SignalWatcher, error) {
	// SystemBus returns a shared connection. It should not be closed.
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %v", err)
	}
	return NewSignalWatcher(ctx, conn, spec)
}

// Close stops watching for signals.
func (sw *SignalWatcher) Close() error {
	// TODO(derat): Check how dbus-daemon handles duplicate matches and document whether multiple
	// SignalWatchers with the same match string can coexist.
	err := sw.conn.BusObject().Call(BusInterface+".RemoveMatch", 0, sw.spec.String()).Err
	sw.conn.RemoveSignal(sw.allSigs)
	close(sw.allSigs)
	close(sw.Signals)
	return err
}

// GetNextSignal returns the next signal on conn that is matched by spec.
func GetNextSignal(ctx context.Context, conn *dbus.Conn, spec MatchSpec) (*dbus.Signal, error) {
	sw, err := NewSignalWatcher(ctx, conn, spec)
	if err != nil {
		return nil, err
	}
	defer sw.Close()

	for {
		select {
		case sig := <-sw.Signals:
			return sig, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
