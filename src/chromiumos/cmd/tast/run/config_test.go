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
	if cfg.checkTestDeps != checkTestDepsAuto {
		t.Errorf("%s set checkTestDeps to %v; want %v", desc, cfg.checkTestDeps, checkTestDepsAuto)
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
	if cfg.checkTestDeps != checkTestDepsNever {
		t.Errorf("%s set checkTestDeps to %v; want %v",
			desc, cfg.checkTestDeps, checkTestDepsNever)
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
}
