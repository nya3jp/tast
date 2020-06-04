// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/testutil"
)

func TestConfigRunDefaults(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	const desc = "SetFlags for RunTestsMode"
	if !cfg.collectSysInfo {
		t.Errorf("%s didn't set collectSysInfo", desc)
	}
	if !cfg.checkTestDeps {
		t.Errorf("%s set checkTestDeps to %v; want true", desc, cfg.checkTestDeps)
	}
}

func TestConfigListDefaults(t *testing.T) {
	cfg := NewConfig(ListTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	desc := "SetFlags for ListTestsMode"
	if cfg.collectSysInfo {
		t.Errorf("%s set collectSysInfo", desc)
	}
	if cfg.checkTestDeps {
		t.Errorf("%s set checkTestDeps to %v; want false", desc, cfg.checkTestDeps)
	}
}

func TestConfigDeriveDefaultsNoBuild(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.build = false

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if !cfg.runLocal {
		t.Error("runLocal is false; want true")
	}
	if !cfg.runRemote {
		t.Error("runRemote is false; want true")
	}
}

func TestConfigDeriveDefaultsBuild(t *testing.T) {
	const buildBundle = "cros"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Create the local bundle package.
	if err := os.MkdirAll(filepath.Join(td, "src/platform/tast-tests/src", localBundlePkgPathPrefix, buildBundle), 0755); err != nil {
		t.Fatal("mkdir failed: ", err)
	}

	cfg := NewConfig(RunTestsMode, "", td)
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.buildBundle = buildBundle

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.buildWorkspace == "" {
		t.Error("buildWorkspace is not set")
	}
	if cfg.localBundleDir == "" {
		t.Error("localBundleDir is not set")
	}
	if !cfg.runLocal {
		t.Error("runLocal is false; want true")
	}
	if cfg.runRemote {
		t.Error("runRemote is true; want false")
	}
}

func TestConfigDeriveDefaultsBuildNonStandardBundle(t *testing.T) {
	const buildBundle = "nonstandardbundle"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Create the remote bundle package.
	if err := os.MkdirAll(filepath.Join(td, "src", remoteBundlePkgPathPrefix, buildBundle), 0755); err != nil {
		t.Fatal("mkdir failed: ", err)
	}

	cfg := NewConfig(RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.buildBundle = buildBundle

	// Since buildBundle is a not known bundle, DeriveDefaults fails to compute
	// buildWorkspace.
	if err := cfg.DeriveDefaults(); err == nil {
		t.Error("DeriveDefaults succeeded; want failure")
	}

	// It works if buildWorkspace is set explicitly.
	cfg.buildWorkspace = td
	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.localBundleDir == "" {
		t.Error("localBundleDir is not set")
	}
	if cfg.runLocal {
		t.Error("runLocal is true; want false")
	}
	if !cfg.runRemote {
		t.Error("runRemote is false; want true")
	}
}

func TestConfigDeriveDefaultsBuildMissingBundle(t *testing.T) {
	const buildBundle = "nosuchbundle"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cfg := NewConfig(RunTestsMode, "", td)
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.buildBundle = buildBundle

	// At least either one of local/remote bundle package should exist.
	if err := cfg.DeriveDefaults(); err == nil {
		t.Error("DeriveDefaults succeeded; want failure")
	}
}

func TestConfigDeriveDefaultsVars(t *testing.T) {
	for _, tc := range []struct {
		name      string
		vars      map[string]string
		overrides map[string]string
		defaults  map[string]string
		defaults2 map[string]string
		want      map[string]string
		wantError bool
	}{
		{
			name: "empty",
			vars: map[string]string{},
			want: map[string]string{},
		},
		{
			name:      "merge",
			vars:      map[string]string{"a": "1"},
			overrides: map[string]string{"override.yaml": "b: 2"},
			defaults:  map[string]string{"default.yaml": "c: 3"},
			want:      map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name:     "var_overrides_default",
			vars:     map[string]string{"a": "1"},
			defaults: map[string]string{"default.yaml": "a: 2"},
			want:     map[string]string{"a": "1"},
		},
		{
			name:      "varsfile_overrides_default",
			vars:      map[string]string{},
			overrides: map[string]string{"override.yaml": "a: 1"},
			defaults:  map[string]string{"default.yaml": "a: 2"},
			want:      map[string]string{"a": "1"},
		},
		{
			name:      "conflict_between_var_and_varsfile",
			vars:      map[string]string{"a": "1"},
			overrides: map[string]string{"override.yaml": "a: 2"},
			wantError: true,
		},
		{
			name: "conflict_within_varsfile",
			vars: map[string]string{},
			overrides: map[string]string{
				"override1.yaml": "a: 1",
				"override2.yaml": "a: 2",
			},
			wantError: true,
		},
		{
			name: "conflict_within_defaults",
			vars: map[string]string{},
			defaults: map[string]string{
				"default1.yaml": "a: 1",
				"default2.yaml": "a: 2",
			},
			wantError: true,
		},
		{
			name:      "multiple_defaults",
			vars:      map[string]string{},
			defaults:  map[string]string{"a.yaml": "a: 1"},
			defaults2: map[string]string{"a.yaml": "b: 2"},
			want:      map[string]string{"a": "1", "b": "2"},
		},
		{
			name:      "multiple_defaults_conflict",
			vars:      map[string]string{},
			defaults:  map[string]string{"a.yaml": "a: 1"},
			defaults2: map[string]string{"a.yaml": "a: 2"},
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			td := testutil.TempDir(t)
			defer os.RemoveAll(td)

			var defaultVarsDirs []string
			for i, m := range []map[string]string{tc.defaults, tc.defaults2} {
				dir := filepath.Join(td, fmt.Sprintf("default_vars%d", i+1))
				defaultVarsDirs = append(defaultVarsDirs, dir)
				if len(m) > 0 {
					os.MkdirAll(dir, 0777)
					if err := testutil.WriteFiles(dir, m); err != nil {
						t.Fatal(err)
					}
				}
			}

			overrideVarsDir := filepath.Join(td, "override_vars")
			os.MkdirAll(overrideVarsDir, 0777)
			if err := testutil.WriteFiles(overrideVarsDir, tc.overrides); err != nil {
				t.Fatal(err)
			}

			cfg := NewConfig(RunTestsMode, "", td)
			flags := flag.NewFlagSet("", flag.ContinueOnError)
			cfg.SetFlags(flags)

			cfg.build = false
			cfg.testVars = tc.vars
			cfg.defaultVarsDirs = defaultVarsDirs
			for p := range tc.overrides {
				cfg.varsFiles = append(cfg.varsFiles, filepath.Join(overrideVarsDir, p))
			}

			if err := cfg.DeriveDefaults(); err != nil {
				if !tc.wantError {
					t.Fatal("DeriveDefaults failed: ", err)
				}
				return
			}
			if tc.wantError {
				t.Fatal("DeriveDefaults unexpectedly succeeded")
			}
			if diff := cmp.Diff(tc.vars, tc.want); diff != "" {
				t.Fatalf("Unexpected vars after DeriveDefaults (-got +want):\n%s", diff)
			}
		})
	}
}

func TestConfigDeriveExtraAllowedBuckets(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.build = false
	cfg.extraAllowedBuckets = []string{"bucket1", "bucket2"}
	cfg.buildArtifactsURL = "gs://bucket3/dir/"

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	want := []string{"bucket1", "bucket2", "bucket3"}
	if got := cfg.extraAllowedBuckets; !reflect.DeepEqual(got, want) {
		t.Errorf("cfg.extraAllowedBuckets = %q; want %q", got, want)
	}
}

func TestConfigLocalBundleGlob(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	cfg.localBundleDir = "/mock/local_bundle_dir"
	cfg.buildBundle = "mock_build_bundle"

	cfg.build = true
	if g := cfg.localBundleGlob(); g != "/mock/local_bundle_dir/mock_build_bundle" {
		t.Fatalf(`Unexpected build localBundleGlob: got %q; want "/mock/local_bundle_dir/mock_bundle_dir"`, g)
	}
	cfg.build = false
	if g := cfg.localBundleGlob(); g != "/mock/local_bundle_dir/*" {
		t.Fatalf(`Unexpected non-build localBundleGlob: got %q; want "/mock/local_bundle_dir/*"`, g)
	}
}

func TestConfigRemoteBundleGlob(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	cfg.remoteBundleDir = "/mock/remote_bundle_dir"
	cfg.buildBundle = "mock_build_bundle"

	cfg.build = true
	if g := cfg.remoteBundleGlob(); g != "/mock/remote_bundle_dir/mock_build_bundle" {
		t.Fatalf(`Unexpected build remoteBundleGlob: got %q; want "/mock/remote_bundle_dir/mock_bundle_dir"`, g)
	}
	cfg.build = false
	if g := cfg.remoteBundleGlob(); g == "/mock/remote_budnle_dir/*" {
		t.Fatalf(`Unexpected non-build remoteBundleGlob: got %q, want "/mock/remote_budnle_dir/*"`, g)
	}
}
