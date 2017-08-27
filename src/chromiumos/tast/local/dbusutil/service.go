// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dbusutil

import (
	"context"

	"github.com/godbus/dbus"
)

// WaitForService blocks until a D-Bus client on conn takes ownership of the name svc.
// If the name is already owned, it returns immediately.
func WaitForService(ctx context.Context, conn *dbus.Conn, svc string) error {
	obj := conn.Object(BusName, BusPath)
	owned := func() bool { return obj.Call(BusInterface+".GetNameOwner", 0, svc).Err == nil }

	// If the name is already owned, we're done.
	if owned() {
		return nil
	}

	sw, err := NewSignalWatcher(ctx, conn, MatchSpec{
		Type:      "signal",
		Path:      BusPath,
		Interface: BusInterface,
		Sender:    BusName,
		Member:    "NameOwnerChanged",
		Arg0:      svc,
	})
	if err != nil {
		return err
	}
	defer sw.Close()

	// Make sure the name wasn't taken while we were creating the watcher.
	if owned() {
		return nil
	}

	for {
		select {
		case sig := <-sw.Signals:
			if len(sig.Body) < 3 {
				continue
			}
			// Skip signals about this service if the "new owner" arg is empty.
			if v, ok := sig.Body[2].(string); !ok || v == "" {
				continue
			}
			// Otherwise, we're done.
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
