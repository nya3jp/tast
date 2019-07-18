// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

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
