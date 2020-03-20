// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"io"
	"reflect"
	"testing"
	"time"
)

func TestProtobufRaw(t *testing.T) {
	r1, w1 := io.Pipe()
	defer r1.Close()
	defer w1.Close()

	// Add extra 5 seconds for the test.
	timeout := time.After(5*time.Second + ioWaitTime)
	done := make(chan bool)
	req1 := &BundleParamsReq{
		Vars: map[string]string{"key1": "value1"},
	}
	req2 := &BundleParamsReq{}
	reqRec1 := &BundleParamsReq{}
	reqRec2 := &BundleParamsReq{}

	go func() {
		for _, req := range []*BundleParamsReq{req1, req2} {
			if err := SendRawMessage(w1, req); err != nil {
				done <- true
				t.Fatalf("Failed to send message %v to stream: %v", req, err)
			}
		}
	}()
	go func() {
		for _, rec := range []*BundleParamsReq{reqRec1, reqRec2} {
			if err := ReceiveRawMessage(r1, rec); err != nil {
				done <- true
				t.Fatalf("Failed to receive message from stream: %v", err)
			}
		}
		if !reflect.DeepEqual(req1.GetVars(), reqRec1.GetVars()) {
			t.Errorf("Received message: %v; want %v", reqRec1.GetVars(), req1.GetVars())
		}
		if !reflect.DeepEqual(req2.GetVars(), reqRec2.GetVars()) {
			t.Errorf("Received message: %v; want %v", reqRec2.GetVars(), req2.GetVars())
		}

		// Make sure receive method can time out.
		reqRec := &BundleParamsReq{}
		if err := ReceiveRawMessage(r1, reqRec); err == nil {
			t.Errorf("Message received from empty stream: %v", reqRec)
		} else if err.Error() != "timed out waiting for protobuf messages" {
			t.Errorf("Timeout error expected. Got: %v", err)
		}
		done <- true
	}()
	select {
	case <-timeout:
		t.Error("Test didn't finish in time.")
	case <-done:
	}
}
