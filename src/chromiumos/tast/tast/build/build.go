// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package build provides support for compiling tests.
package build

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"chromiumos/tast/tast/timing"
)

const (
	sysGopath = "/usr/lib/gopath" // readonly Go workspace where source for system packages are stored
)

// GetLocalArch returns the local system's architecture as described by "uname -m".
func GetLocalArch() (string, error) {
	b, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// archToCompiler maps from a machine name (or processor, see "uname -m") to the corresponding
// Go command that should be used for building.
// TODO(derat): What's the right way to get the toolchain name for a given board?
// "cros_setup_toolchains --show-board-cfg <board>" seems to print it, but it's very slow (700+ ms).
var archToCompiler map[string]string = map[string]string{
	"i686":    "i686-pc-linux-gnu-go",
	"x86_64":  "x86_64-cros-linux-gnu-go",
	"armv7l":  "armv7a-cros-linux-gnueabi-go",
	"aarch64": "armv7a-cros-linux-gnueabi-go",
}

// BuildTests builds executable package pkg to path as dictated by cfg.
func BuildTests(ctx context.Context, cfg *Config, pkg, path string) (out []byte, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("build_tests")
		defer st.End()
	}

	comp := archToCompiler[cfg.Arch]
	if comp == "" {
		return out, fmt.Errorf("unknown arch %q", cfg.Arch)
	}

	pkgDir := filepath.Join(cfg.OutDir, cfg.Arch)
	cmd := exec.Command(comp, "build", "-i", "-ldflags=-s -w", "-pkgdir", pkgDir, "-o", path, pkg)
	cmd.Env = []string{
		"PATH=/usr/bin",
		"GOPATH=" + strings.Join([]string{cfg.TestWorkspace, sysGopath}, ":"),
	}
	if out, err = cmd.CombinedOutput(); err != nil {
		return out, err
	}
	return out, nil
}
