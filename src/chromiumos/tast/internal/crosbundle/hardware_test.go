// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"testing"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/internal/protocol"
)

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
		soc   protocol.DeprecatedDeviceConfig_SOC
	}{
		{
			lscpu("142", "Intel(R) Core(TM) m3-8100Y CPU @ 1.10GHz"),
			protocol.DeprecatedDeviceConfig_SOC_AMBERLAKE_Y,
		},
		{
			lscpu("142", "Intel(R) Core(TM) m3-7Y30 Processor @ 2.60GHz"),
			protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_Y,
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

func TestFindMemorySize(t *testing.T) {
	testCases := []struct {
		input         []byte
		expectSuccess bool
		expectedSize  int64
	}{
		{
			[]byte(`MemTotal:       987654321 kB
			MemFree:         99999999 kB
			`), true, 987654321000,
		},
		{
			[]byte(`
			MemFree:       11111111 kB
			MemTotal:      12345678 kB
			`), true, 12345678000,
		},
		{
			[]byte(``), false, 0,
		},
	}
	for _, tc := range testCases {
		r, err := findMemorySize(tc.input)
		if !tc.expectSuccess {
			if err == nil {
				t.Errorf("Unexpectedly succeeded: input=%s", string(tc.input))
			}
			continue
		}
		if err != nil {
			t.Errorf("Failed to find memory size: %v; input=%s", err, string(tc.input))
			continue
		}
		if r != tc.expectedSize {
			t.Errorf("Got %d, want %d; input=%s", r, tc.expectedSize, string(tc.input))
		}
	}
}

func TestFindSpeakerAmplifier(t *testing.T) {
	testCases := []struct {
		input  string
		expect string
	}{
		{
			"MX98357A:00",
			configpb.HardwareFeatures_Audio_MAX98357.String(),
		},
		{
			"max98357a",
			configpb.HardwareFeatures_Audio_MAX98357.String(),
		},
		{
			"max98357a_1",
			configpb.HardwareFeatures_Audio_MAX98357.String(),
		},
		{
			"i2c-mx98357a:00",
			configpb.HardwareFeatures_Audio_MAX98357.String(),
		},
		{
			"i2c-MX98373:00",
			configpb.HardwareFeatures_Audio_MAX98373.String(),
		},
		{
			"MX98360A",
			configpb.HardwareFeatures_Audio_MAX98360.String(),
		},
		{
			"RTL1015",
			configpb.HardwareFeatures_Audio_RT1015.String(),
		},
		{
			"i2c-10EC1015:00",
			configpb.HardwareFeatures_Audio_RT1015.String(),
		},
		{
			"rt1015.6-0028",
			configpb.HardwareFeatures_Audio_RT1015.String(),
		},
		{
			"rt1015p",
			configpb.HardwareFeatures_Audio_RT1015P.String(),
		},
		{
			"rt1015p_1",
			configpb.HardwareFeatures_Audio_RT1015P.String(),
		},
		{
			"i2c-10EC1011:02",
			configpb.HardwareFeatures_Audio_ALC1011.String(),
		},
		{
			"i2c-MX98390:00",
			configpb.HardwareFeatures_Audio_MAX98390.String(),
		},
	}
	for _, tc := range testCases {
		amp, match := matchSpeakerAmplifier(tc.input)
		if !match {
			t.Errorf("Failed to match amplifer; input=%s", string(tc.input))
			continue
		}
		if amp.GetName() != tc.expect {
			t.Errorf("Got %s, expect %s: input=%s", amp.GetName(), tc.expect, tc.input)
		}
	}
}
