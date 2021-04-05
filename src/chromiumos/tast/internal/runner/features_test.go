// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/testutil"
)

func TestGetDUTInfo(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"use_flags":  "# here's a comment\nfoo\nbar\n",
		"lsbrelease": "CHROMEOS_RELEASE_BOARD=betty\n",
	}); err != nil {
		t.Fatal(err)
	}

	osVersion := "octopus-release/R86-13312.0.2020_07_02_1108"
	cfg := Config{
		Type:           LocalRunner,
		USEFlagsFile:   filepath.Join(td, "use_flags"),
		LSBReleaseFile: filepath.Join(td, "lsbrelease"),
		SoftwareFeatureDefinitions: map[string]string{
			"foobar":       "foo && bar",
			"not_foo":      "!foo",
			"other":        "baz",
			"foo_glob":     "\"f*\"",
			"not_bar_glob": "!\"b*\"",
			"board":        `"board:betty"`,
			"not_board":    `"board:eve"`,
		},
		OSVersion: osVersion,
	}
	status, stdout, _, sig := callRun(
		t, nil,
		&jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerGetDUTInfoMode,
			GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
				ExtraUSEFlags: []string{"baz"},
			},
		},
		nil, &cfg)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}
	var res jsonprotocol.RunnerGetDUTInfoResult
	if err := json.NewDecoder(stdout).Decode(&res); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	exp := jsonprotocol.RunnerGetDUTInfoResult{
		SoftwareFeatures: &protocol.SoftwareFeatures{
			Available:   []string{"board", "foo_glob", "foobar", "other"},
			Unavailable: []string{"not_bar_glob", "not_board", "not_foo"},
		},
		OSVersion: osVersion,
	}
	if !reflect.DeepEqual(res, exp) {
		t.Errorf("%v wrote result %+v; want %+v", sig, res, exp)
	}
}

func TestGetSoftwareFeaturesNoFile(t *testing.T) {
	// If the file listing USE flags was missing, an empty result should be returned.
	cfg := Config{
		Type:                       LocalRunner,
		USEFlagsFile:               "/tmp/nonexistent_use_flags_file.txt",
		SoftwareFeatureDefinitions: map[string]string{"foo": "bar"},
	}
	args := &jsonprotocol.RunnerArgs{
		Mode:       jsonprotocol.RunnerGetDUTInfoMode,
		GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{},
	}
	status, stdout, _, sig := callRun(t, nil, args, nil, &cfg)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}
	var res jsonprotocol.RunnerGetDUTInfoResult
	if err := json.NewDecoder(stdout).Decode(&res); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	exp := jsonprotocol.RunnerGetDUTInfoResult{}
	if !reflect.DeepEqual(res, exp) {
		t.Errorf("%v wrote result %+v; want %+v", sig, res, exp)
	}
}

func TestDetermineSoftwareFeatures(t *testing.T) {
	defs := map[string]string{"a": "foo && bar", "b": "foo && baz"}
	flags := []string{"foo", "bar"}
	autotestCaps := map[string]autocaps.State{"c": autocaps.Yes, "d": autocaps.No, "e": autocaps.Disable}
	features, err := determineSoftwareFeatures(defs, flags, autotestCaps)
	if err != nil {
		t.Fatalf("determineSoftwareFeatures(%v, %v, %v) failed: %v", defs, flags, autotestCaps, err)
	}
	if exp := []string{"a", autotestCapPrefix + "c"}; !reflect.DeepEqual(features.Available, exp) {
		t.Errorf("determineSoftwareFeatures(%v, %v, %v) returned available features %v; want %v",
			defs, flags, autotestCaps, features.Available, exp)
	}
	if exp := []string{autotestCapPrefix + "d", autotestCapPrefix + "e", "b"}; !reflect.DeepEqual(features.Unavailable, exp) {
		t.Errorf("determineSoftwareFeatures(%v, %v, %v) returned unavailable features %v; want %v",
			defs, flags, autotestCaps, features.Unavailable, exp)
	}
}

func TestGetDiskSize(t *testing.T) {
	testCases := []struct {
		input         []byte
		expectSuccess bool
		expectedSize  int64
	}{
		{
			// a real data from Caroline, with some partitions removed
			// lsblk from util-linux 2.32
			[]byte(`{
			"blockdevices": [
				{"name": "loop0", "maj:min": "7:0", "rm": "0", "size": "7876964352", "ro": "0", "type": "loop", "mountpoint": null,
					"children": [
						{"name": "encstateful", "maj:min": "253:1", "rm": "0", "size": "7876964352", "ro": "0", "type": "dm", "mountpoint": "/mnt/stateful_partition/encrypted"}
					]
				},
				{"name": "loop1", "maj:min": "7:1", "rm": "0", "size": "420405248", "ro": "1", "type": "loop", "mountpoint": "/opt/google/containers/android/rootfs/root"},
				{"name": "loop2", "maj:min": "7:2", "rm": "0", "size": "4096", "ro": "1", "type": "loop", "mountpoint": "/opt/google/containers/arc-sdcard/mountpoints/container-root"},
				{"name": "loop3", "maj:min": "7:3", "rm": "0", "size": "4096", "ro": "1", "type": "loop", "mountpoint": "/opt/google/containers/arc-obb-mounter/mountpoints/container-root"},
				{"name": "loop4", "maj:min": "7:4", "rm": "0", "size": "111665152", "ro": "1", "type": "loop", "mountpoint": "/usr/share/chromeos-assets/speech_synthesis/patts"},
				{"name": "loop5", "maj:min": "7:5", "rm": "0", "size": "51994624", "ro": "1", "type": "loop", "mountpoint": null},
				{"name": "loop8", "maj:min": "7:8", "rm": "0", "size": "9670656", "ro": "1", "type": "loop", "mountpoint": "/usr/share/chromeos-assets/quickoffice/_platform_specific"},
				{"name": "mmcblk0", "maj:min": "179:0", "rm": "0", "size": "31268536320", "ro": "0", "type": "disk", "mountpoint": null,
					"children": [
						{"name": "mmcblk0p1", "maj:min": "179:1", "rm": "0", "size": "26812063744", "ro": "0", "type": "part", "mountpoint": "/mnt/stateful_partition"},
						{"name": "mmcblk0p2", "maj:min": "179:2", "rm": "0", "size": "16777216", "ro": "0", "type": "part", "mountpoint": null},
						{"name": "mmcblk0p3", "maj:min": "179:3", "rm": "0", "size": "2147483648", "ro": "0", "type": "part", "mountpoint": null},
						{"name": "mmcblk0p12", "maj:min": "179:12", "rm": null, "size": "33554432", "ro": "0", "type": "part", "mountpoint": null}
					]
				},
				{"name": "mmcblk0boot0", "maj:min": "179:16", "rm": "0", "size": "4194304", "ro": "1", "type": "disk", "mountpoint": null},
				{"name": "mmcblk0boot1", "maj:min": "179:32", "rm": "0", "size": "4194304", "ro": "1", "type": "disk", "mountpoint": null},
				{"name": "mmcblk0rpmb", "maj:min": "179:48", "rm": "0", "size": "4194304", "ro": "0", "type": "disk", "mountpoint": null},
				{"name": "zram0", "maj:min": "252:0", "rm": "0", "size": "5907427328", "ro": "0", "type": "disk", "mountpoint": "[SWAP]"}
			]
		}`),
			true,
			31268536320,
		},
		{
			// a real data from Eve, with some partitions removed
			[]byte(`{
				"blockdevices": [
				{"name": "loop0", "maj:min": "7:0", "rm": "0", "size": "148315029504", "ro": "0", "type": "loop", "mountpoint": null,
					"children": [
						{"name": "encstateful", "maj:min": "254:1", "rm": "0", "size": "148315029504", "ro": "0", "type": "dm", "mountpoint": "/mnt/stateful_partition/encrypted"}
					]
				},
				{"name": "loop1", "maj:min": "7:1", "rm": "0", "size": "512675840", "ro": "1", "type": "loop", "mountpoint": "/opt/google/containers/android/rootfs/root"},
				{"name": "loop2", "maj:min": "7:2", "rm": "0", "size": "4096", "ro": "1", "type": "loop", "mountpoint": "/run/chromeos-config/private"},
				{"name": "loop3", "maj:min": "7:3", "rm": "0", "size": "4096", "ro": "1", "type": "loop", "mountpoint": "/opt/google/containers/arc-sdcard/mountpoints/container-root"},
				{"name": "loop4", "maj:min": "7:4", "rm": "0", "size": "4096", "ro": "1", "type": "loop", "mountpoint": "/opt/google/containers/arc-obb-mounter/mountpoints/container-root"},
				{"name": "loop5", "maj:min": "7:5", "rm": "0", "size": "111665152", "ro": "1", "type": "loop", "mountpoint": "/usr/share/chromeos-assets/speech_synthesis/patts"},
				{"name": "loop6", "maj:min": "7:6", "rm": "0", "size": "102952960", "ro": "1", "type": "loop", "mountpoint": null},
				{"name": "loop9", "maj:min": "7:9", "rm": "0", "size": "9670656", "ro": "1", "type": "loop", "mountpoint": "/usr/share/chromeos-assets/quickoffice/_platform_specific"},
				{"name": "zram0", "maj:min": "253:0", "rm": "0", "size": "24439869440", "ro": "0", "type": "disk", "mountpoint": "[SWAP]"},
				{"name": "nvme0n1", "maj:min": "259:0", "rm": "0", "size": "512110190592", "ro": "0", "type": "disk", "mountpoint": null,
					"children": [
						{"name": "nvme0n1p1", "maj:min": "259:1", "rm": "0", "size": "503358750720", "ro": "0", "type": "part", "mountpoint": "/mnt/stateful_partition"},
						{"name": "nvme0n1p2", "maj:min": "259:2", "rm": "0", "size": "16777216", "ro": "0", "type": "part", "mountpoint": null},
						{"name": "nvme0n1p3", "maj:min": "259:3", "rm": "0", "size": "4294967296", "ro": "0", "type": "part", "mountpoint": null},
						{"name": "nvme0n1p12", "maj:min": "259:12", "rm": "0", "size": "33554432", "ro": "0", "type": "part", "mountpoint": null}
					]
				}
				]
			}`),
			true,
			512110190592,
		},
		{
			[]byte(`{
				"blockdevices": [
					{"name": "realdisk", "rm": "0", "size": "1", "type": "disk"},
					{"name": "realdisk2", "rm": "0", "size": "2", "type": "disk"},
					{"name": "loop", "rm": null, "size": "4", "ro": "0", "type": "loop"},
					{"name": "removable", "rm": "1", "size": "8", "type": "disk"},
					{"name": "zram0", "rm": "0", "size": "16", "type": "disk"}
				]
			}`),
			true,
			2,
		},
		{
			[]byte(`{
				"blockdevices": [
					{"name": "zram0", "rm": "0", "size": "8", "type": "disk"}
				]
			}`),
			// no disk found
			false,
			0,
		},
		{
			// lsblk from util-linux 2.36.1
			// size is number, rm is boolean instead of strings.
			[]byte(`{
				"blockdevices": [
					{"name":"sda", "maj:min":"8:0", "rm":false, "size":2147483648000, "ro":false, "type":"disk", "mountpoint":null,
						"children": [
							{"name":"sda1", "maj:min":"8:1", "rm":false, "size":2000000000, "ro":false, "type":"part", "mountpoint":"/boot/efi"},
							{"name":"sda2", "maj:min":"8:2", "rm":false, "size":2000000000, "ro":false, "type":"part", "mountpoint":"/boot"},
							{"name":"sda3", "maj:min":"8:3", "rm":false, "size":2143482582528, "ro":false, "type":"part", "mountpoint":null,
								"children": [
									{"name":"myhost-root", "maj:min":"254:0", "rm":false, "size":2141477404672, "ro":false, "type":"lvm", "mountpoint":"/"},
									{"name":"myhost-swap", "maj:min":"254:1", "rm":false, "size":2000683008, "ro":false, "type":"lvm", "mountpoint":"[SWAP]"}
								]
							}
						]
					}
				]
			}`),
			true,
			2147483648000,
		},
	}
	for _, tc := range testCases {
		r, err := findDiskSize(tc.input)
		if !tc.expectSuccess {
			if err == nil {
				t.Errorf("Unexpectedly succeeded: input=%s", string(tc.input))
			}
			continue
		}
		if err != nil {
			t.Errorf("Failed to find disk size: %v; input=%s", err, string(tc.input))
			continue
		}
		if r != tc.expectedSize {
			t.Errorf("Got %d, want %d; input=%s", r, tc.expectedSize, string(tc.input))
		}
	}
}

func TestFindIntelSOC(t *testing.T) {
	lscpu := func(model, modelName string) *lscpuResult {
		return &lscpuResult{
			Entries: []lscpuEntry{
				{Field: "CPU family:", Data: "6"},
				{Field: "Model:", Data: model},
				{Field: "Model name:", Data: modelName},
			},
		}
	}
	testCases := []struct {
		input *lscpuResult
		soc   device.Config_SOC
	}{
		{
			lscpu("142", "Intel(R) Core(TM) m3-8100Y CPU @ 1.10GHz"),
			device.Config_SOC_AMBERLAKE_Y,
		},
		{
			lscpu("142", "Intel(R) Core(TM) m3-7Y30 Processor @ 2.60GHz"),
			device.Config_SOC_KABYLAKE_Y,
		},
	}
	for _, tc := range testCases {
		r, err := findIntelSOC(tc.input)
		if err != nil {
			t.Errorf("Failed to find Intel SoC for %v: %s", *tc.input, err)
		}
		if r != tc.soc {
			t.Errorf("Got %s, want %s: input=%v", r, tc.soc, *tc.input)
		}
	}
}
