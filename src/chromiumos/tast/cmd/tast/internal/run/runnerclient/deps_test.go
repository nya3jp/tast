// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
)

// writeGetDUTInfoResult writes runner.RunnerGetDUTInfoResult to w.
func writeGetDUTInfoResult(w io.Writer, avail, unavail []string, dc *device.Config, hf *configpb.HardwareFeatures, osVersion, defaultBuildArtifactsURL string) error {
	res := jsonprotocol.RunnerGetDUTInfoResult{
		SoftwareFeatures: &protocol.SoftwareFeatures{
			Available:   avail,
			Unavailable: unavail,
		},
		DeviceConfig:             dc,
		HardwareFeatures:         hf,
		OSVersion:                osVersion,
		DefaultBuildArtifactsURL: defaultBuildArtifactsURL,
	}
	return json.NewEncoder(w).Encode(&res)
}

// checkRunnerTestDepsArgs calls featureArgsFromConfig using cfg and verifies
// that it sets runner args as specified per checkDeps, avail, and unavail.
func checkRunnerTestDepsArgs(t *testing.T, cfg *config.Config, state *config.State, checkDeps bool,
	avail, unavail []string, dc *device.Config, hf *configpb.HardwareFeatures) {
	t.Helper()
	args := jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerRunTestsMode,
		RunTests: &jsonprotocol.RunnerRunTestsArgs{
			BundleArgs: jsonprotocol.BundleRunTestsArgs{
				FeatureArgs: *featureArgsFromConfig(cfg, state),
			},
		},
	}

	exp := jsonprotocol.RunnerRunTestsArgs{
		BundleArgs: jsonprotocol.BundleRunTestsArgs{
			FeatureArgs: jsonprotocol.FeatureArgs{
				CheckSoftwareDeps:           checkDeps,
				AvailableSoftwareFeatures:   avail,
				UnavailableSoftwareFeatures: unavail,
				DeviceConfig: jsonprotocol.DeviceConfigJSON{
					Proto: dc,
				},
				HardwareFeatures: jsonprotocol.HardwareFeaturesJSON{
					Proto: hf,
				},
			},
		},
	}
	if !cmp.Equal(*args.RunTests, exp, cmp.Comparer(proto.Equal)) {
		t.Errorf("featureArgsFromConfig(%+v) set %+v; want %+v", cfg, *args.RunTests, exp)
	}
}

func TestGetDUTInfo(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// With "always", features returned by the runner should be passed through
	// and dependencies should be checked.
	avail := []string{"dep1", "dep2"}
	unavail := []string{"dep3"}
	dc := &device.Config{
		Id: &device.ConfigId{
			PlatformId: &device.PlatformId{Value: "platform-id"},
			ModelId:    &device.ModelId{Value: "model-id"},
			BrandId:    &device.BrandId{Value: "brand-id"},
		},
	}
	hf := &configpb.HardwareFeatures{
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
	}
	osVersion := "octopus-release/R86-13312.0.2020_07_02_1108"
	defaultBuildArtifactsURL := "gs://chromeos-image-archive/octopus-release/R86-13312.0.2020_07_02_1108/"
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerGetDUTInfoMode,
			GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
				ExtraUSEFlags:       td.Cfg.ExtraUSEFlags,
				RequestDeviceConfig: true,
			},
		})

		writeGetDUTInfoResult(stdout, avail, unavail, dc, hf, osVersion, defaultBuildArtifactsURL)
		return 0
	}
	td.Cfg.CheckTestDeps = true
	td.Cfg.ExtraUSEFlags = []string{"use1", "use2"}

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if err := GetDUTInfo(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("GetDUTInfo(%+v) failed: %v", td.Cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.Cfg, &td.State, true, avail, unavail, dc, hf)

	if td.State.OSVersion != osVersion {
		t.Errorf("Unexpected OS version: got %+v, want %+v", td.State.OSVersion, osVersion)
	}

	if td.State.DefaultBuildArtifactsURL != defaultBuildArtifactsURL {
		t.Errorf("Unexpected DefaultBuildArtifactsURL: got %+v, want %+v", td.State.DefaultBuildArtifactsURL, defaultBuildArtifactsURL)
	}

	// Make sure device-config.txt is created.
	if b, err := ioutil.ReadFile(filepath.Join(td.Cfg.ResDir, "device-config.txt")); err != nil {
		t.Error("Failed to read device-config.txt: ", err)
	} else {
		var readDc device.Config
		if err := proto.UnmarshalText(string(b), &readDc); err != nil {
			t.Error("Failed to unmarshal device config: ", err)
		} else if !proto.Equal(dc, &readDc) {
			t.Errorf("Unexpected device config: got %+v, want %+v", &readDc, dc)
		}
	}

	// The second call should fail, because it tries to update cfg's fields twice.
	if err := GetDUTInfo(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Fatal("Calling GetDUTInfo twice unexpectedly succeeded")
	}
}

func TestGetDUTInfoNoDeviceConfig(t *testing.T) {
	// If local_test_runner is older, it may not return device.Config even if it is requested.
	// For backward compatibility, it is not handled as an error case, but the device-config.txt
	// won't be created.
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerGetDUTInfoMode,
			GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
				ExtraUSEFlags:       td.Cfg.ExtraUSEFlags,
				RequestDeviceConfig: true,
			},
		})

		// Note: if both avail/unavail are empty, it is handled as an error.
		// Add fake here to avoid it.
		writeGetDUTInfoResult(stdout, []string{"dep1"}, nil, nil, nil, "", "")
		return 0
	}
	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if err := GetDUTInfo(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("GetDUTInfo(%+v) failed: %v", td.Cfg, err)
	}

	// Make sure device-config.txt is created.
	if _, err := os.Stat(filepath.Join(td.Cfg.ResDir, deviceConfigFile)); err == nil || !os.IsNotExist(err) {
		t.Error("Unexpected device config file: ", err)
	}
}

func TestGetDUTInfoNoCheckTestDeps(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
	td.Cfg.CheckTestDeps = false

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if err := GetDUTInfo(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("GetDUTInfo(%+v) failed: %v", td.Cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.Cfg, &td.State, false, nil, nil, nil, nil)
}

func TestGetSoftwareFeaturesNoFeatures(t *testing.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()
	// "always" should fail if the runner doesn't know about any features.
	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerGetDUTInfoMode,
			GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
				RequestDeviceConfig: true,
			},
		})
		writeGetDUTInfoResult(stdout, []string{}, []string{}, nil, nil, "", "")
		return 0
	}
	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if err := GetDUTInfo(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Fatalf("getSoftwareFeatures(%+v) succeeded unexpectedly", td.Cfg)
	}
}