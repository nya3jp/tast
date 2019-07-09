// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"flag"
	"testing"
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

func TestConfigDeriveDefaults(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.buildBundle = "cros"
	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.buildWorkspace == "" {
		t.Error("buildWorkspace is not set")
	}
	if cfg.localBundleDir == "" {
		t.Error("localBundleDir is not set")
	}
}

func TestConfigDeriveDefaultsNonStandardBundle(t *testing.T) {
	cfg := NewConfig(RunTestsMode, "", "")
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cfg.SetFlags(flags)

	cfg.buildBundle = "nonstandardbundle"
	if err := cfg.DeriveDefaults(); err == nil {
		t.Error("DeriveDefaults succeeded; want failure")
	}

	cfg.buildWorkspace = "/path/to/workspace"
	if err := cfg.DeriveDefaults(); err != nil {
		t.Error("DeriveDefaults failed: ", err)
	}
	if cfg.localBundleDir == "" {
		t.Error("localBundleDir is not set")
	}
}
