// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package build provides support for compiling tests.
package build

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/tast/timing"
)

const (
	buildRoot     = "/build"                            // base chroot directory containing sysroots
	baseSysGopath = "/usr/lib/gopath"                   // readonly Go workspace where package source is stored
	commonSrcDir  = "src/chromiumos/tast/common/runner" // directory expected to exist within baseSysGopath
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

// getNewestSysGopath attempts to find the most-recently-updated Go workspace containing
// source code for tests' dependencies. Source code is checked out to usr/lib/gopath/
// (relative to the root for the host system, and relative to /build/<board> for other
// boards) when the corresponding Portage packages are built.
func getNewestSysGopath() string {
	var newestPath string
	var newestTime time.Time
	if fi, err := os.Stat(filepath.Join(baseSysGopath, commonSrcDir)); err == nil {
		newestPath = baseSysGopath
		newestTime = fi.ModTime()
	}

	if ds, err := ioutil.ReadDir(buildRoot); err == nil {
		for _, di := range ds {
			dir := filepath.Join(buildRoot, di.Name(), baseSysGopath)
			if fi, err := os.Stat(filepath.Join(dir, commonSrcDir)); err == nil {
				if newestTime.IsZero() || fi.ModTime().After(newestTime) {
					newestPath = dir
					newestTime = fi.ModTime()
				}
			}
		}
	}
	return newestPath
}

// BuildTests builds executable package pkg to path as dictated by cfg.
func BuildTests(ctx context.Context, cfg Config, pkg, path string) (out []byte, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("build_tests")
		defer st.End()
	}

	comp := archToCompiler[cfg.Arch]
	if comp == "" {
		return out, fmt.Errorf("unknown arch %q", cfg.Arch)
	}

	if cfg.SysGopath == "" {
		if cfg.SysGopath = getNewestSysGopath(); cfg.SysGopath == "" {
			return out, errors.New("failed to find source for test dependencies -- " +
				"emerge chromeos-base/tast-{local,remote}-tests?")
		}
		cfg.Logger.Debug("Found system GOPATH %s; use -sysgopath to override", cfg.SysGopath)
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
