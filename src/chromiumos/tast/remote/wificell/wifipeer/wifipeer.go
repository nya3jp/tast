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
//type WifiPeer struct {
//Peers []*ssh.Conn
//}

// MakePeers constructs the peer devices needed for the tests.
func MakePeers(ctx context.Context, testdut *dut.DUT, count int) (peers []*ssh.Conn, retErr error) {
	defer func() {
		if retErr != nil {
			for _, peer := range peers {
				peer.Close(ctx)
			}
		}
	}()
	for i := 0; i < count; i++ {
		if newDut, err := testdut.WifiPeerHost(ctx, i); err != nil {
			return nil, err
			peers = append(peers, newDut)
		}
	}
	return peers, nil
}
