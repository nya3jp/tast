// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"flag"
	"os"
	"testing"
)

func TestConfigRunDefaults(t *testing.T) {
	// setEnv sets name to val and returns a func to restore the original value.
	setEnv := func(name, val string) (undo func()) {
		orig := os.Getenv(name)
		if err := os.Setenv(name, val); err != nil {
			t.Fatal(err)
		}
		return func() {
			if orig == "" {
				os.Unsetenv(name)
			} else {
				os.Setenv(name, orig)
			}
		}
	}

	// Proxy-related flags' defaults should come from environment variables.
	const (
		httpProxy  = "10.0.0.1:8000"
		httpsProxy = "10.0.0.1:8001"
		noProxy    = "foo.com, bar.com"
	)
	defer setEnv("HTTP_PROXY", httpProxy)()
	defer setEnv("HTTPS_PROXY", httpsProxy)()
	defer setEnv("NO_PROXY", noProxy)()

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
	if cfg.httpProxy != httpProxy {
		t.Errorf("%s set httpProxy to %q; want %q", desc, cfg.httpProxy, httpProxy)
	}
	if cfg.httpsProxy != httpsProxy {
		t.Errorf("%s set httpsProxy to %q; want %q", desc, cfg.httpsProxy, httpsProxy)
	}
	if cfg.noProxy != noProxy {
		t.Errorf("%s set noProxy to %q; want %q", desc, cfg.noProxy, noProxy)
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
