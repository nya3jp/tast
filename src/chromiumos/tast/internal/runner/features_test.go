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
	"chromiumos/tast/testutil"
)

func TestGetDUTInfo(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"use_flags": "# here's a comment\nfoo\nbar\n",
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
		SoftwareFeatures: &SoftwareFeatures{
			Available:   []string{"foo_glob", "foobar", "other"},
			Unavailable: []string{"not_bar_glob", "not_foo"},
		},
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
	exp := GetDUTInfoResult{}
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
