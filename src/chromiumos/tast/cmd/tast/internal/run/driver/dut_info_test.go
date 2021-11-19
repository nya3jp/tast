// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/protocol"
)

func TestDriver_GetDUTInfo(t *testing.T) {
	want := &protocol.DUTInfo{
		Features: &protocol.DUTFeatures{
			Software: &protocol.SoftwareFeatures{
				Available:   []string{"dep1", "dep2"},
				Unavailable: []string{"dep3"},
			},
			Hardware: &protocol.HardwareFeatures{
				HardwareFeatures: &configpb.HardwareFeatures{
					Screen: &configpb.HardwareFeatures_Screen{
						PanelProperties: &configpb.Component_DisplayPanel_Properties{},
						TouchSupport:    configpb.HardwareFeatures_PRESENT,
					},
					Fingerprint: &configpb.HardwareFeatures_Fingerprint{
						Location: configpb.HardwareFeatures_Fingerprint_NOT_PRESENT,
					},
					EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
						Present: configpb.HardwareFeatures_PRESENT,
						EcType:  configpb.HardwareFeatures_EmbeddedController_EC_CHROME,
					},
				},
				DeprecatedDeviceConfig: &protocol.DeprecatedDeviceConfig{
					Id: &protocol.DeprecatedConfigId{
						Platform: "platform_id",
						Model:    "model_id",
						Brand:    "brand_id",
					},
				},
			},
		},
		OsVersion:                "octopus-release/R86-13312.0.2020_07_02_1108",
		DefaultBuildArtifactsUrl: "gs://chromeos-image-archive/octopus-release/R86-13312.0.2020_07_02_1108/",
	}
	extraUseFlags := []string{"use1", "use2"}

	env := runtest.SetUp(t, runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
		if diff := cmp.Diff(req.GetExtraUseFlags(), extraUseFlags); diff != "" {
			t.Errorf("ExtraUseFlags mismatch (-got +want):\n%s", diff)
		}
		return &protocol.GetDUTInfoResponse{DutInfo: want}, nil
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.CheckTestDeps = true
		cfg.ExtraUSEFlags = extraUseFlags
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	got, err := drv.GetDUTInfo(ctx)
	if err != nil {
		t.Fatalf("GetDUTInfo failed: %v", err)
	}

	if diff := cmp.Diff(got, want, cmp.Comparer(proto.Equal)); diff != "" {
		t.Errorf("DUTInfo mismatch (-got +want):\n%s", diff)
	}
}

func TestDriver_GetDUTInfo_NoCheckTestDeps(t *testing.T) {
	env := runtest.SetUp(t, runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
		t.Error("GetDUTInfo called unexpectedly")
		return &protocol.GetDUTInfoResponse{}, nil
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
		cfg.CheckTestDeps = false
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if _, err := drv.GetDUTInfo(ctx); err != nil {
		t.Fatalf("GetDUTInfo failed: %v", err)
	}
}
