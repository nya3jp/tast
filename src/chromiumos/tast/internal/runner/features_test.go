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
	scfg := StaticConfig{
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
		nil, &scfg)
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
	scfg := StaticConfig{
		Type:                       LocalRunner,
		USEFlagsFile:               "/tmp/nonexistent_use_flags_file.txt",
		SoftwareFeatureDefinitions: map[string]string{"foo": "bar"},
	}
	args := &jsonprotocol.RunnerArgs{
		Mode:       jsonprotocol.RunnerGetDUTInfoMode,
		GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{},
	}
	status, stdout, _, sig := callRun(t, nil, args, nil, &scfg)
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
