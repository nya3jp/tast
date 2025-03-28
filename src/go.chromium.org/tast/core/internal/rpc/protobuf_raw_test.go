// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast/core/internal/protocol"
)

func TestProtobufRaw(t *testing.T) {
	var buf bytes.Buffer

	// Send two messages.
	msgSent1 := &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			Vars: map[string]string{"key1": "value1"},
		},
	}
	msgSent2 := &protocol.HandshakeResponse{
		Error: &protocol.HandshakeError{
			Reason: "error happened",
		},
	}
	for _, req := range []proto.Message{msgSent1, msgSent2} {
		if err := sendRawMessage(&buf, req); err != nil {
			t.Fatalf("Failed to send message %v to stream: %v", req, err)
		}
	}

	// Receive messages and compare.
	msgReceived1 := &protocol.HandshakeRequest{}
	msgReceived2 := &protocol.HandshakeResponse{}
	for _, reqRead := range []proto.Message{msgReceived1, msgReceived2} {
		if err := receiveRawMessage(&buf, reqRead); err != nil {
			t.Fatalf("Failed to receive message from stream: %v", err)
		}
	}
	if !proto.Equal(msgSent1, msgReceived1) {
		t.Errorf("Received message: %v; want %v", msgReceived1, msgSent1)
	}
	if !proto.Equal(msgSent2, msgReceived2) {
		t.Errorf("Received message: %v; want %v", msgReceived2, msgSent2)
	}
}
