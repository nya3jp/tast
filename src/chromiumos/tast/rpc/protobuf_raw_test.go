// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"io"
	"sync"
	"testing"

	proto "github.com/golang/protobuf/proto"
)

func TestProtobufRaw(t *testing.T) {
	// Send both request and response in a single pipe.
	r, w := io.Pipe()

	msgWrite1 := &InitBundleServerRequest{
		Vars: map[string]string{"key1": "value1"},
	}
	msgRead1 := &InitBundleServerRequest{}

	msgWrite2 := &InitBundleServerResponse{
		Success:      false,
		ErrorMessage: "error happened",
	}
	msgRead2 := &InitBundleServerResponse{}

	var wg sync.WaitGroup

	// Send two messages on pipe.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer w.Close() // Close the pipe so receiver can quit.
		for _, req := range []proto.Message{msgWrite1, msgWrite2} {
			if err := sendRawMessage(w, req); err != nil {
				t.Fatalf("Failed to send message %v to stream: %v", req, err)
			}
		}
	}()

	// Receive messages from pipe and compare.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer r.Close() // Close the pipe so sender can quit.
		for _, reqRead := range []proto.Message{msgRead1, msgRead2} {
			if err := receiveRawMessage(r, reqRead); err != nil {
				t.Fatalf("Failed to receive message from stream: %v", err)
			}
		}
		if !proto.Equal(msgWrite1, msgRead1) {
			t.Errorf("Received message: %v; want %v", msgRead1, msgWrite1)
		}
		if !proto.Equal(msgWrite2, msgRead2) {
			t.Errorf("Received message: %v; want %v", msgRead2, msgWrite2)
		}
	}()

	wg.Wait()
}
