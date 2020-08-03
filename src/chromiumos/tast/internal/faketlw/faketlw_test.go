// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package faketlw

import (
	"context"
	"testing"

	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"
)

func TestWiringServer_OpenDutPort(t *testing.T) {
	ctx := context.Background()

	src := NamePort{Name: "foo", Port: 1234}
	dst := NamePort{Name: "bar", Port: 2345}
	srv, addr := StartWiringServer(t, WithDUTPortMap(map[NamePort]NamePort{src: dst}))
	defer srv.Stop()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to Dial: ", err)
	}
	defer conn.Close()

	cl := tls.NewWiringClient(conn)

	req := &tls.OpenDutPortRequest{Name: src.Name, Port: src.Port}
	res, err := cl.OpenDutPort(ctx, req)
	if err != nil {
		t.Errorf("OpenDutPort(%q, %d): %v", req.Name, req.Port, err)
	} else if res.GetAddress() != dst.Name || res.GetPort() != dst.Port {
		t.Errorf("OpenDutPort(%q, %d) = (%q, %d); want (%q, %d)", req.Name, req.Port, res.GetAddress(), res.GetPort(), dst.Name, dst.Port)
	}

	req = &tls.OpenDutPortRequest{Name: src.Name, Port: 9999}
	res, err = cl.OpenDutPort(ctx, req)
	if err == nil {
		t.Errorf("OpenDutPort(%q, %d): unexpectedly succeeded", req.Name, req.Port)
	}
}
