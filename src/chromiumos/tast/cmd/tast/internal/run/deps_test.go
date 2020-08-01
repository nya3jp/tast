// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

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

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/runner"
)

// writeGetDUTInfoResult writes runner.GetDUTInfoResult to w.
func writeGetDUTInfoResult(w io.Writer, avail, unavail []string, dc *device.Config, hf *configpb.HardwareFeatures, osVersion string) error {
	res := runner.GetDUTInfoResult{
		SoftwareFeatures: &dep.SoftwareFeatures{
			Available:   avail,
			Unavailable: unavail,
		},
		DeviceConfig:     dc,
		HardwareFeatures: hf,
		OSVersion:        osVersion,
	}
	return json.NewEncoder(w).Encode(&res)
}

// checkRunnerTestDepsArgs calls setRunnerTestDepsArgs using cfg and verifies
// that it sets runner args as specified per checkDeps, avail, and unavail.
func checkRunnerTestDepsArgs(t *testing.T, cfg *Config, checkDeps bool,
	avail, unavail []string, dc *device.Config, hf *configpb.HardwareFeatures, osVersion string) {
	t.Helper()
	args := runner.Args{
		Mode:     runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{},
	}
	setRunnerTestDepsArgs(cfg, &args)

	exp := runner.RunTestsArgs{
		BundleArgs: bundle.RunTestsArgs{
			CheckSoftwareDeps:           checkDeps,
			AvailableSoftwareFeatures:   avail,
			UnavailableSoftwareFeatures: unavail,
			DeviceConfig:                dc,
			HardwareFeatures:            hf,
		},
	}
	if !cmp.Equal(*args.RunTests, exp, cmp.Comparer(proto.Equal)) {
		t.Errorf("setRunnerTestDepsArgs(%+v) set %+v; want %+v", cfg, *args.RunTests, exp)
	}
}

func TestGetDUTInfo(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

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
			TouchSupport: configpb.HardwareFeatures_PRESENT,
		},
	}
	osVersion := "octopus-release/R86-13312.0.2020_07_02_1108"
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.GetDUTInfoMode,
			GetDUTInfo: &runner.GetDUTInfoArgs{
				ExtraUSEFlags:       td.cfg.extraUSEFlags,
				RequestDeviceConfig: true,
			},
		})

		writeGetDUTInfoResult(stdout, avail, unavail, dc, hf, osVersion)
		return 0
	}
	td.cfg.checkTestDeps = true
	td.cfg.extraUSEFlags = []string{"use1", "use2"}
	if err := getDUTInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getDUTInfo(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, true, avail, unavail, dc, hf, osVersion)

	// Make sure device-config.txt is created.
	if b, err := ioutil.ReadFile(filepath.Join(td.cfg.ResDir, "device-config.txt")); err != nil {
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
	if err := getDUTInfo(context.Background(), &td.cfg); err == nil {
		t.Fatal("Calling getDUTInfo twice unexpectedly succeeded")
	}
}

func TestGetDUTInfoNoDeviceConfig(t *testing.T) {
	// If local_test_runner is older, it may not return device.Config even if it is requested.
	// For backward compatibility, it is not handled as an error case, but the device-config.txt
	// won't be created.
	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.GetDUTInfoMode,
			GetDUTInfo: &runner.GetDUTInfoArgs{
				ExtraUSEFlags:       td.cfg.extraUSEFlags,
				RequestDeviceConfig: true,
			},
		})

		// Note: if both avail/unavail are empty, it is handled as an error.
		// Add dummy here to avoid it.
		writeGetDUTInfoResult(stdout, []string{"dep1"}, nil, nil, nil, "")
		return 0
	}
	td.cfg.checkTestDeps = true
	if err := getDUTInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getDUTInfo(%+v) failed: %v", td.cfg, err)
	}

	// Make sure device-config.txt is created.
	if _, err := os.Stat(filepath.Join(td.cfg.ResDir, deviceConfigFile)); err == nil || !os.IsNotExist(err) {
		t.Error("Unexpected device config file: ", err)
	}
}

func TestGetDUTInfoNoCheckTestDeps(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
	td.cfg.checkTestDeps = false
	if err := getDUTInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getDUTInfo(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil, nil, nil, "")
}

func TestGetSoftwareFeaturesNoFeatures(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()
	// "always" should fail if the runner doesn't know about any features.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.GetDUTInfoMode,
			GetDUTInfo: &runner.GetDUTInfoArgs{
				RequestDeviceConfig: true,
			},
		})
		writeGetDUTInfoResult(stdout, []string{}, []string{}, nil, nil, "")
		return 0
	}
	td.cfg.checkTestDeps = true
	if err := getDUTInfo(context.Background(), &td.cfg); err == nil {
		t.Fatalf("getSoftwareFeatures(%+v) succeeded unexpectedly", td.cfg)
	}
}
