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
			"foobar":  "foo && bar",
			"not_foo": "!foo",
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
		Available:   []string{"foobar"},
		Unavailable: []string{"not_foo"},
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
