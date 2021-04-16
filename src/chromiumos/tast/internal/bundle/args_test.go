// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/internal/jsonprotocol"
)

// newBufferWithArgs returns a bytes.Buffer containing the JSON representation of args.
func newBufferWithArgs(t *testing.T, args *jsonprotocol.BundleArgs) *bytes.Buffer {
	t.Helper()
	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(args); err != nil {
		t.Fatal(err)
	}
	return &b
}

func TestReadArgs(t *testing.T) {
	want := &jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleListTestsMode,
		ListTests: &jsonprotocol.BundleListTestsArgs{
			Patterns: []string{"example.*"},
		},
	}
	got, err := readArgs(nil, newBufferWithArgs(t, want), ioutil.Discard)
	if err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatal("BundleArgs mismatch (-got +want): ", diff)
	}
}

func TestReadArgsDumpTests(t *testing.T) {
	got, err := readArgs([]string{"-dumptests"}, &bytes.Buffer{}, ioutil.Discard)
	if err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	want := &jsonprotocol.BundleArgs{
		Mode:      jsonprotocol.BundleListTestsMode,
		ListTests: &jsonprotocol.BundleListTestsArgs{},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatal("BundleArgs mismatch (-got +want): ", diff)
	}
}

func TestReadArgsRPC(t *testing.T) {
	got, err := readArgs([]string{"-rpc"}, &bytes.Buffer{}, ioutil.Discard)
	if err != nil {
		t.Fatal("readArgs failed: ", err)
	}

	want := &jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleRPCMode,
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatal("BundleArgs mismatch (-got +want): ", diff)
	}
}

func TestMarshal(t *testing.T) {
	// 0-bytes data after marshal is treated as nil.
	// Fill some fields to test non-nil case here.
	in := &jsonprotocol.BundleRunTestsArgs{
		FeatureArgs: jsonprotocol.FeatureArgs{
			AvailableSoftwareFeatures:   []string{"feature1"},
			UnavailableSoftwareFeatures: []string{"feature2"},
			DeviceConfig: jsonprotocol.DeviceConfigJSON{
				Proto: &device.Config{
					Id: &device.ConfigId{
						PlatformId: &device.PlatformId{Value: "platformId"},
						ModelId:    &device.ModelId{Value: "modelId"},
						BrandId:    &device.BrandId{Value: "brandId"},
					},
				},
			},
			HardwareFeatures: jsonprotocol.HardwareFeaturesJSON{
				Proto: &configpb.HardwareFeatures{
					Screen: &configpb.HardwareFeatures_Screen{
						TouchSupport: configpb.HardwareFeatures_PRESENT,
						PanelProperties: &configpb.Component_DisplayPanel_Properties{
							DiagonalMilliinch: 11000,
						},
					},
					Fingerprint: &configpb.HardwareFeatures_Fingerprint{
						Location: configpb.HardwareFeatures_Fingerprint_NOT_PRESENT,
					},
					EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
						Present: configpb.HardwareFeatures_NOT_PRESENT,
						EcType:  configpb.HardwareFeatures_EmbeddedController_EC_TYPE_UNKNOWN,
						Part:    &configpb.Component_EmbeddedController{PartNumber: "my_part_number"},
					},
				},
			},
		},
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal("Failed to marshalize JSON:", err)
	}
	out := &jsonprotocol.BundleRunTestsArgs{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal("Failed to unmarshal JSON: ", err)
	}
	if !proto.Equal(in.DeviceConfig.Proto, out.DeviceConfig.Proto) {
		t.Errorf("DeviceConfig did not match: want %v, got %v", in.DeviceConfig.Proto, out.DeviceConfig.Proto)
	}
	if !proto.Equal(in.HardwareFeatures.Proto, out.HardwareFeatures.Proto) {
		t.Errorf("HardwareFeatures did not match: want %v, got %v", in.HardwareFeatures.Proto, out.HardwareFeatures.Proto)
	}
	if !reflect.DeepEqual(in.AvailableSoftwareFeatures, out.AvailableSoftwareFeatures) {
		t.Errorf("AvailableSoftwareFeatures did not match: want %v, got %v", in.AvailableSoftwareFeatures, out.AvailableSoftwareFeatures)
	}
	if !reflect.DeepEqual(in.UnavailableSoftwareFeatures, out.UnavailableSoftwareFeatures) {
		t.Errorf("UnavailableSoftwareFeatures did not match: want %v, got %v", in.UnavailableSoftwareFeatures, out.UnavailableSoftwareFeatures)
	}
}
