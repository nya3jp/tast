// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	configpb "go.chromium.org/chromiumos/config/go/api"
	metadatapb "go.chromium.org/chromiumos/config/go/api/test/metadata/v1"
	"go.chromium.org/chromiumos/infra/proto/go/device"
)

// newBufferWithArgs returns a bytes.Buffer containing the JSON representation of args.
func newBufferWithArgs(t *testing.T, args *Args) *bytes.Buffer {
	t.Helper()
	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(args); err != nil {
		t.Fatal(err)
	}
	return &b
}

func TestReadArgs(t *testing.T) {
	const (
		defaultDataDir = "/mock/data"
		pattern        = "example.*"
	)

	args := &Args{
		RunTests: &RunTestsArgs{
			DataDir: defaultDataDir,
		},
	}
	stdin := newBufferWithArgs(t, &Args{
		Mode: ListTestsMode,
		ListTests: &ListTestsArgs{
			Patterns: []string{pattern},
		},
	})
	if err := readArgs(nil, stdin, ioutil.Discard, args, localBundle); err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	// Args are merged.
	exp := &Args{
		Mode: ListTestsMode,
		RunTests: &RunTestsArgs{
			DataDir: defaultDataDir,
		},
		ListTests: &ListTestsArgs{
			Patterns: []string{pattern},
		},
	}
	if diff := cmp.Diff(args, exp); diff != "" {
		t.Fatal("Args mismatch (-want +got): ", diff)
	}
}

func TestReadArgsDumpTests(t *testing.T) {
	args := &Args{}
	if err := readArgs([]string{"-dumptests"}, &bytes.Buffer{}, ioutil.Discard, args, localBundle); err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	exp := &Args{
		Mode:      ListTestsMode,
		ListTests: &ListTestsArgs{},
	}
	if diff := cmp.Diff(args, exp); diff != "" {
		t.Fatal("Args mismatch (-want +got): ", diff)
	}
}

func TestReadArgsRPC(t *testing.T) {
	args := &Args{}
	if err := readArgs([]string{"-rpc"}, &bytes.Buffer{}, ioutil.Discard, args, localBundle); err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	exp := &Args{
		Mode: RPCMode,
	}
	if diff := cmp.Diff(args, exp); diff != "" {
		t.Fatal("Args mismatch (-want +got): ", diff)
	}
}

func TestMarshal(t *testing.T) {
	in := &RunTestsArgs{
		DeviceConfig: &device.Config{},
		DUT: &metadatapb.DUTConfigConstraint_DUT{
			HardwareFeatures: &configpb.HardwareFeatures{
				Screen: &configpb.HardwareFeatures_Screen{
					TouchSupport: configpb.HardwareFeatures_PRESENT,
				},
			},
		},
	}
	b, err := in.MarshalJSON()
	if err != nil {
		t.Fatal("Failed to marshalize JSON")
	}
	out := &RunTestsArgs{}
	out.UnmarshalJSON(b)
	if !proto.Equal(in.DeviceConfig, out.DeviceConfig) {
		t.Error("DeviceConfig did not match")
	}
	if !proto.Equal(in.DUT, out.DUT) {
		t.Error("DUT did not match")
	}
	if diff := cmp.Diff(in, out); diff != "" {
		t.Error("In/out mismatch (-want +got): ", diff)
	}
}
