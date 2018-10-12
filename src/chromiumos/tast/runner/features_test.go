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

func TestGetSoftwareFeatures(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"use_flags": "# here's a comment\nfoo\nbar\n",
	}); err != nil {
		t.Fatal(err)
	}

	defaultArgs := Args{
		USEFlagsFile: filepath.Join(td, "use_flags"),
		SoftwareFeatureDefinitions: map[string]string{
			"foobar":       "foo && bar",
			"not_foo":      "!foo",
			"other":        "baz",
			"foo_glob":     "\"f*\"",
			"not_bar_glob": "!\"b*\"",
		},
		GetSoftwareFeaturesArgs: GetSoftwareFeaturesArgs{
			ExtraUSEFlags: []string{"baz"},
		},
	}
	status, stdout, _, sig := callRun(t, nil, &Args{Mode: GetSoftwareFeaturesMode}, &defaultArgs, LocalRunner)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}
	var res GetSoftwareFeaturesResult
	if err := json.NewDecoder(stdout).Decode(&res); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	exp := GetSoftwareFeaturesResult{
		Available:   []string{"foo_glob", "foobar", "other"},
		Unavailable: []string{"not_bar_glob", "not_foo"},
	}
	if !reflect.DeepEqual(res, exp) {
		t.Errorf("%v wrote result %+v; want %+v", sig, res, exp)
	}
}

func TestGetSoftwareFeaturesNoFile(t *testing.T) {
	// If the file listing USE flags was missing, an empty result should be returned.
	defaultArgs := Args{
		USEFlagsFile:               "/tmp/nonexistent_use_flags_file.txt",
		SoftwareFeatureDefinitions: map[string]string{"foo": "bar"},
	}
	status, stdout, _, sig := callRun(t, nil, &Args{Mode: GetSoftwareFeaturesMode}, &defaultArgs, LocalRunner)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}
	var res GetSoftwareFeaturesResult
	if err := json.NewDecoder(stdout).Decode(&res); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	exp := GetSoftwareFeaturesResult{}
	if !reflect.DeepEqual(res, exp) {
		t.Errorf("%v wrote result %+v; want %+v", sig, res, exp)
	}
}

func TestDetermineSoftwareFeatures(t *testing.T) {
	defs := map[string]string{"a": "foo && bar", "b": "foo && baz"}
	flags := []string{"foo", "bar"}
	autotestCaps := map[string]autocaps.State{"c": autocaps.Yes, "d": autocaps.No, "e": autocaps.Disable}
	avail, unavail, err := determineSoftwareFeatures(defs, flags, autotestCaps)
	if err != nil {
		t.Fatalf("determineSoftwareFeatures(%v, %v, %v) failed: %v", defs, flags, autotestCaps, err)
	}
	if exp := []string{"a", autotestCapPrefix + "c"}; !reflect.DeepEqual(avail, exp) {
		t.Errorf("determineSoftwareFeatures(%v, %v, %v) returned available features %v; want %v",
			defs, flags, autotestCaps, avail, exp)
	}
	if exp := []string{autotestCapPrefix + "d", autotestCapPrefix + "e", "b"}; !reflect.DeepEqual(unavail, exp) {
		t.Errorf("determineSoftwareFeatures(%v, %v, %v) returned unavailable features %v; want %v",
			defs, flags, autotestCaps, unavail, exp)
	}
}
