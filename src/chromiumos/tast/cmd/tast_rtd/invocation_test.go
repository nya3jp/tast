// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"testing"

	"github.com/golang/protobuf/proto"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

// TestUnmarshalInvocation makes sure unmarshalInvocation able to unmarshal invocation data.
func TestUnmarshalInvocation(t *testing.T) {
	inv := rtd.Invocation{
		Requests: []*rtd.Request{
			{
				Name: "Name1",
				Test: "cros.Test1",
				Environment: &rtd.Request_Environment{
					WorkDir: "/tmp/tast",
				},
			},
			{
				Name: "Name2",
				Test: "cros.Test2",
				Environment: &rtd.Request_Environment{
					WorkDir: "/tmp/tast",
				},
			},
		},
		ProgressSinkClientConfig: &rtd.ProgressSinkClientConfig{
			Port: 22,
		},
		TestLabServicesConfig: &rtd.TLSClientConfig{
			TlsAddress: "127.0.0.1",
			TlsPort:    2222,
			TlwAddress: "127.0.0.1",
			TlwPort:    2223,
		},
		Duts: []*rtd.DUT{
			{
				TlsDutName: "127.0.0.1:2224",
			},
		},
	}
	buf, err := proto.Marshal(&inv)
	if err != nil {
		t.Fatal("Failed to marshal invocation data:", err)
	}
	result, err := unmarshalInvocation(buf)
	if err != nil {
		t.Fatal("Failed to unmarshal invocation data:", err)
	}
	if !proto.Equal(&inv, result) {
		t.Errorf("Invocation did not match: want %v, got %v", inv, result)
	}
}
