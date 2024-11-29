// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	configpb "go.chromium.org/chromiumos/config/go/api"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/cmd/tast/internal/run/runtest"
	"go.chromium.org/tast/core/internal/protocol"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

func TestDriver_GetDUTInfo(t *testing.T) {
	want := &protocol.DUTInfo{
		Features: &frameworkprotocol.DUTFeatures{
			Software: &frameworkprotocol.SoftwareFeatures{
				Available:   []string{"dep1", "dep2"},
				Unavailable: []string{"dep3"},
			},
			Hardware: &frameworkprotocol.HardwareFeatures{
				HardwareFeatures: &configpb.HardwareFeatures{
					Screen: &configpb.HardwareFeatures_Screen{
						PanelProperties: &configpb.Component_DisplayPanel_Properties{},
						TouchSupport:    configpb.HardwareFeatures_PRESENT,
					},
					Fingerprint: &configpb.HardwareFeatures_Fingerprint{
						Present: false,
					},
					EmbeddedController: &configpb.HardwareFeatures_EmbeddedController{
						Present: configpb.HardwareFeatures_PRESENT,
						EcType:  configpb.HardwareFeatures_EmbeddedController_EC_CHROME,
					},
				},
				DeprecatedDeviceConfig: &frameworkprotocol.DeprecatedDeviceConfig{
					Id: &frameworkprotocol.DeprecatedConfigId{
						Platform: "platform_id",
						Model:    "model_id",
						Brand:    "brand_id",
					},
				},
			},
		},
		OsVersion:                "octopus-release/R107-15117.5.0",
		DefaultBuildArtifactsUrl: "gs://chromeos-image-archive/octopus-release/R107-15117.5.0/",
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

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
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

func TestDriver_GetDUTInfo_NoCheckTestDepsForRun(t *testing.T) {
	env := runtest.SetUp(t, runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
		if req.GetFeatures() {
			t.Error("GetDUTInfo request should not check for features")
		}
		return &protocol.GetDUTInfoResponse{}, nil
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
		cfg.CheckTestDeps = false
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if _, err := drv.GetDUTInfo(ctx); err != nil {
		t.Fatalf("GetDUTInfo failed: %v", err)
	}
}

func TestDriver_GetDUTInfo_NoTestDepsForList(t *testing.T) {
	env := runtest.SetUp(t, runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
		if req.GetFeatures() {
			t.Error("GetDUTInfo request should not check for features")
		}
		return &protocol.GetDUTInfoResponse{}, nil
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		// The dependencies shouldn't be checked for listing tests.
		cfg.Mode = config.ListTestsMode
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if _, err := drv.GetDUTInfo(ctx); err != nil {
		t.Fatalf("GetDUTInfo failed: %v", err)
	}
}

func TestDriverGetDUTInfoNoHost(t *testing.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Target = "-"
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)
	gotInfo, err := drv.GetDUTInfo(ctx)
	if err != nil {
		t.Fatalf("GetDUTInfo failed: %v", err)
	}
	if gotInfo != nil {
		t.Fatalf("GetDUTInfo failed: got %v want nil", gotInfo)
	}
}
