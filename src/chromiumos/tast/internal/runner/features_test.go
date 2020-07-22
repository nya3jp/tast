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

	"chromiumos/tast/autocaps"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/testutil"
)

func TestGetDUTInfo(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"use_flags": "# here's a comment\nfoo\nbar\n",
		"lsb-release": "CHROMEOS_RELEASE_APPID={9A3BE5D2-C3DC-4AE6-9943-E2C113895DC5}\n" +
			"CHROMEOS_BOARD_APPID={9A3BE5D2-C3DC-4AE6-9943-E2C113895DC5}\n" +
			"CHROMEOS_CANARY_APPID={90F229CE-83E2-4FAF-8479-E368A34938B1}\n" +
			"DEVICETYPE=CHROMEBOOK\n" +
			"GOOGLE_RELEASE=13312.0.2020_07_02_1108\n" +
			"CHROMEOS_RELEASE_BOARD=octopus\n" +
			"CHROMEOS_RELEASE_BRANCH_NUMBER=0\n" +
			"CHROMEOS_RELEASE_TRACK=testimage-channel\n" +
			"CHROMEOS_RELEASE_KEYSET=devkeys\n" +
			"CHROMEOS_RELEASE_NAME=Chromium OS\n" +
			"CHROMEOS_AUSERVER=http://abc.mtv.corp.google.com:8080/updatev\n" +
			"CHROMEOS_ARC_VERSION=6633360\n" +
			"CHROMEOS_ARC_ANDROID_SDK_VERSION=28\n" +
			"CHROMEOS_DEVSERVER=http://abc.mtv.corp.google.com:8080\n" +
			"CHROMEOS_RELEASE_BUILD_NUMBER=13312\n" +
			"CHROMEOS_RELEASE_CHROME_MILESTONE=86\n" +
			"CHROMEOS_RELEASE_PATCH_NUMBER=2020_07_02_1108\n" +
			"CHROMEOS_RELEASE_BUILD_TYPE=Test Build - abc\n" +
			"CHROMEOS_RELEASE_UNIBUILD=1\n" +
			"CHROMEOS_RELEASE_VERSION=13312.0.2020_07_02_1108\n" +
			"CHROMEOS_RELEASE_DESCRIPTION=13312.0.2020_07_02_1108 (Test Build - abc) developer-build xyz\n",
	}); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Type:         LocalRunner,
		USEFlagsFile: filepath.Join(td, "use_flags"),
		SoftwareFeatureDefinitions: map[string]string{
			"foobar":       "foo && bar",
			"not_foo":      "!foo",
			"other":        "baz",
			"foo_glob":     "\"f*\"",
			"not_bar_glob": "!\"b*\"",
		},
		LSBReleaseFile: filepath.Join(td, "lsb-release"),
	}
	status, stdout, _, sig := callRun(
		t, nil,
		&Args{
			Mode: GetDUTInfoMode,
			GetDUTInfo: &GetDUTInfoArgs{
				ExtraUSEFlags: []string{"baz"},
			},
		},
		nil, &cfg)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}
	var res GetDUTInfoResult
	if err := json.NewDecoder(stdout).Decode(&res); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	exp := GetDUTInfoResult{
		SoftwareFeatures: &dep.SoftwareFeatures{
			Available:   []string{"foo_glob", "foobar", "other"},
			Unavailable: []string{"not_bar_glob", "not_foo"},
		},
		ChromeOSVersion: "13312.0.2020_07_02_1108",
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
	args := &Args{
		Mode:       GetDUTInfoMode,
		GetDUTInfo: &GetDUTInfoArgs{},
	}
	status, stdout, _, sig := callRun(t, nil, args, nil, &cfg)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}
	var res GetDUTInfoResult
	if err := json.NewDecoder(stdout).Decode(&res); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	chromeOSVersionWarning := "Chrome OS Version is not available because lsb-release does not exist on target"
	exp := GetDUTInfoResult{
		Warnings: []string{chromeOSVersionWarning},
	}
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
