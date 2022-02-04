// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/testutil"
)

func TestMutableConfigRunDefaults(t *testing.T) {
	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	const desc = "SetFlags for RunTestsMode"
	if !cfg.CollectSysInfo {
		t.Errorf("%s didn't set CollectSysInfo", desc)
	}
	if !cfg.CheckTestDeps {
		t.Errorf("%s set CheckTestDeps to %v; want true", desc, cfg.CheckTestDeps)
	}
}

func TestMutableConfigListDefaults(t *testing.T) {
	cfg := config.NewMutableConfig(config.ListTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	desc := "SetFlags for ListTestsMode"
	if cfg.CollectSysInfo {
		t.Errorf("%s set CollectSysInfo", desc)
	}
	if !cfg.CheckTestDeps {
		t.Errorf("%s set CheckTestDeps to %v; want true", desc, cfg.CheckTestDeps)
	}
}

func TestMutableConfigDeriveDefaultsNoBuild(t *testing.T) {
	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.Build = false

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
}

func TestMutableConfigDeriveDefaultsSystemServicesTimeout(t *testing.T) {
	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.Build = false

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.SystemServicesTimeout.Seconds() != 120 {
		t.Errorf("DeriveDefault failed to set default value of SystemServicesTimeout. Expected %f seconds, Found: %f seconds", 120.0, cfg.SystemServicesTimeout.Seconds())
	}
}

func TestMutableConfigDeriveDefaultsBuild(t *testing.T) {
	const buildBundle = "cros"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cfg := config.NewMutableConfig(config.RunTestsMode, "", td)
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.BuildBundle = buildBundle

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.BuildWorkspace == "" {
		t.Error("BuildWorkspace is not set")
	}
	if cfg.LocalBundleDir == "" {
		t.Error("LocalBundleDir is not set")
	}
	if cfg.RemoteBundleDir == "" {
		t.Error("RemoteBundleDir is not set")
	}
}

func TestMutableConfigDeriveDefaultsBuildNonStandardBundle(t *testing.T) {
	const buildBundle = "nonstandardbundle"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.BuildBundle = buildBundle

	// Since buildBundle is a not known bundle, DeriveDefaults fails to compute
	// BuildWorkspace.
	if err := cfg.DeriveDefaults(); err == nil {
		t.Error("DeriveDefaults succeeded; want failure")
	}

	// It works if BuildWorkspace is set explicitly.
	cfg.BuildWorkspace = td
	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.LocalBundleDir == "" {
		t.Error("LocalBundleDir is not set")
	}
	if cfg.LocalBundleDir == "" {
		t.Error("LocalBundleDir is not set")
	}
	if cfg.RemoteBundleDir == "" {
		t.Error("RemoteBundleDir is not set")
	}
}

func TestMutableConfigDeriveDefaultsBuildMissingBundle(t *testing.T) {
	const buildBundle = "nosuchbundle"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cfg := config.NewMutableConfig(config.RunTestsMode, "", td)
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.BuildBundle = buildBundle

	// At least either one of local/remote bundle package should exist.
	if err := cfg.DeriveDefaults(); err == nil {
		t.Error("DeriveDefaults succeeded; want failure")
	}
}

func TestMutableConfigDeriveDefaultsVars(t *testing.T) {
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
		{
			name: "defaults_deep",
			vars: map[string]string{},
			defaults: map[string]string{
				"x/a.yaml": "a: 1",
				"y/a.yaml": "b: 2",
			},
			want: map[string]string{"a": "1", "b": "2"},
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

			cfg := config.NewMutableConfig(config.RunTestsMode, "", td)
			flags := flag.NewFlagSet("", flag.ContinueOnError)
			cfg.SetFlags(flags)

			cfg.Build = false
			cfg.TestVars = tc.vars
			cfg.DefaultVarsDirs = defaultVarsDirs
			for p := range tc.overrides {
				cfg.VarsFiles = append(cfg.VarsFiles, filepath.Join(overrideVarsDir, p))
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

func TestMutableConfigDeriveExtraAllowedBuckets(t *testing.T) {
	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.Build = false
	cfg.ExtraAllowedBuckets = []string{"bucket1", "bucket2"}
	cfg.BuildArtifactsURLOverride = "gs://bucket3/dir/"

	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	want := []string{"bucket1", "bucket2", "bucket3"}
	if got := cfg.ExtraAllowedBuckets; !reflect.DeepEqual(got, want) {
		t.Errorf("cfg.ExtraAllowedBuckets() = %q; want %q", got, want)
	}
}

func TestConfigLocalBundleGlob(t *testing.T) {
	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	cfg.LocalBundleDir = "/mock/local_bundle_dir"
	cfg.BuildBundle = "mock_build_bundle"

	cfg.Build = true
	if g := cfg.Freeze().LocalBundleGlob(); g != "/mock/local_bundle_dir/mock_build_bundle" {
		t.Fatalf(`Unexpected build LocalBundleGlob: got %q; want "/mock/local_bundle_dir/mock_bundle_dir"`, g)
	}
	cfg.Build = false
	if g := cfg.Freeze().LocalBundleGlob(); g != "/mock/local_bundle_dir/*" {
		t.Fatalf(`Unexpected non-build LocalBundleGlob: got %q; want "/mock/local_bundle_dir/*"`, g)
	}
}

func TestConfigRemoteBundleGlob(t *testing.T) {
	cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
	cfg.RemoteBundleDir = "/mock/remote_bundle_dir"
	cfg.BuildBundle = "mock_build_bundle"

	cfg.Build = true
	if g := cfg.Freeze().RemoteBundleGlob(); g != "/mock/remote_bundle_dir/mock_build_bundle" {
		t.Fatalf(`Unexpected build RemoteBundleGlob: got %q; want "/mock/remote_bundle_dir/mock_bundle_dir"`, g)
	}
	cfg.Build = false
	if g := cfg.Freeze().RemoteBundleGlob(); g == "/mock/remote_budnle_dir/*" {
		t.Fatalf(`Unexpected non-build RemoteBundleGlob: got %q, want "/mock/remote_budnle_dir/*"`, g)
	}
}

func TestConfigAttachDebugger(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		want      map[debugger.DebugTarget]int
		wantError bool
	}{
		{
			name: "empty",
			args: []string{},
			want: map[debugger.DebugTarget]int{
				debugger.LocalBundle:  0,
				debugger.RemoteBundle: 0,
			},
		}, {
			name: "single",
			args: []string{"-attachdebugger=local:2345"},
			want: map[debugger.DebugTarget]int{
				debugger.LocalBundle:  2345,
				debugger.RemoteBundle: 0,
			},
		}, {
			name: "many",
			args: []string{"-attachdebugger=local:2345", "-attachdebugger=remote:2346"},
			want: map[debugger.DebugTarget]int{
				debugger.LocalBundle:  2345,
				debugger.RemoteBundle: 2346,
			},
		}, {
			name:      "invalid-name-only",
			args:      []string{"-attachdebugger=local"},
			wantError: true,
		}, {
			name:      "invalid-port-only",
			args:      []string{"-attachdebugger=:2345"},
			wantError: true,
		}, {
			name:      "invalid-unknown-target",
			args:      []string{"-attachdebugger=unknown:2348"},
			wantError: true,
		}, {
			name:      "invalid-mutually-exclusive",
			args:      []string{"-attachdebugger=local:2345", "-build=false"},
			wantError: true,
		},
	} {
		cfg := config.NewMutableConfig(config.RunTestsMode, "", "")
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		cfg.SetFlags(flags)
		reflect.DeepEqual("string", []string{})

		err := flags.Parse(tc.args)
		if err == nil {
			err = cfg.DeriveDefaults()
		}
		if tc.wantError {
			if err == nil {
				t.Fatalf(`Expected an error to be thrown in %s, but it succeeded`, tc.name)
			}
		} else {
			if err != nil {
				t.Fatalf(`Unexpected error in %s: %s`, tc.name, err.Error())
			}
			if got := cfg.DebuggerPorts; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("cfg.DebuggerPorts = %q; want %q", got, tc.want)
			}
		}
	}
}
