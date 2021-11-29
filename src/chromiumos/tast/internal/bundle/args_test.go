// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/base64"
	"io/ioutil"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/protocol"
)

func TestReadArgsRpcTcpServer(t *testing.T) {
	port := 3333
	var req *protocol.HandshakeRequest = &protocol.HandshakeRequest{
		NeedUserServices: true,
		BundleInitParams: &protocol.BundleInitParams{
			BundleConfig: &protocol.BundleConfig{
				PrimaryTarget: &protocol.TargetDevice{
					BundleDir: "BLAR",
				},
			},
		},
	}

	want := &parsedArgs{
		mode:      modeRPCTCP,
		port:      port,
		handshake: req,
	}

	raw, err := proto.Marshal(req)
	if err != nil {
		t.Fatal("Fail to serialize proto: ", err)
	}
	handshakeBase64 := base64.StdEncoding.EncodeToString(raw)
	args := []string{"-rpctcp", "-port", strconv.Itoa(port), "-handshake", handshakeBase64}

	got, err := readArgs(args, ioutil.Discard)
	if err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	if got.mode != want.mode {
		t.Errorf("mdoe = %v, want %v", got.mode, want.mode)
	}
	if got.port != want.port {
		t.Errorf("port = %v, want %v", got.port, want.port)
	}
	if diff := cmp.Diff(got.handshake, want.handshake, protocmp.Transform()); diff != "" {
		t.Errorf("BundleArgs mismatch (-got +want):\n%s", diff)
	}
}
