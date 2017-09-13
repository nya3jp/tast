// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package build provides support for compiling tests.
package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/tast/timing"
)

const (
	baseTastSrcDir = "src/chromiumos/tast" // dir in system GOPATH containing tast source
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

// latestModTime returns the latest modification time among files within dir.
// Errors are ignored.
func latestModTime(dir string) time.Time {
	var latestTime time.Time
	wf := func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.ModTime().After(latestTime) {
			latestTime = fi.ModTime()
		}
		return nil
	}
	filepath.Walk(dir, wf)
	return latestTime
}

// FreshestSysGopath attempts to find the most-recently-updated Go workspace containing
// source code for tests' dependencies among the directories matched by wildcard pattern.
// Source code is checked out to usr/lib/gopath/ (relative to the root for the host system,
// and relative to /build/<board> for other boards) when Portage packages are installed.
func FreshestSysGopath(pattern string) (string, error) {
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	} else if len(dirs) == 0 {
		return "", fmt.Errorf("%q didn't match any directories", pattern)
	}

	var bestDir string
	var bestTime time.Time
	for _, dir := range dirs {
		if t := latestModTime(filepath.Join(dir, baseTastSrcDir)); bestTime.IsZero() || t.After(bestTime) {
			bestDir = dir
			bestTime = t
		}
	}
	return bestDir, nil
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
		"GOPATH=" + strings.Join([]string{cfg.TestWorkspace, cfg.SysGopath}, ":"),
	}
	if out, err = cmd.CombinedOutput(); err != nil {
		return out, err
	}
	return out, nil
}
