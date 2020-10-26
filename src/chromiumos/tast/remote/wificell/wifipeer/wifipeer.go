// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wifipeer builds and controls peer chrome OS devices for wifi tests.
package wifipeer

import (
	"context"

	"chromiumos/tast/dut"
	"chromiumos/tast/ssh"
)

// WifiPeer holds the structure of a Chrome OS peer device for wifi tests.
type WifiPeer struct {
	Peers []*ssh.Conn
}

// MakePeers constructs the peer devices needed for the tests.
func (peer *WifiPeer) MakePeers(ctx context.Context, testdut *dut.DUT,
	count int) (_ []*ssh.Conn, retErr error) {
	for i := 0; i < count; i++ {
		newDut, _ := testdut.IthWifiPeerHost(ctx, i)
		peer.Peers = append(peer.Peers, newDut)
	}
	return peer.Peers, nil //TODO(hinton): Return any errors from the loop.
}
