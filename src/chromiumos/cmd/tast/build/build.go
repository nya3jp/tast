// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package build provides support for compiling tests and related executables.
package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"chromiumos/cmd/tast/timing"
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

// Build builds executable package pkg to outDir as dictated by cfg.
// The executable file's name is assigned by "go install" (i.e. it's the last component of pkg).
// stageName is used as the name of a new stage reported via the timing package.
func Build(ctx context.Context, cfg *Config, pkg, outDir, stageName string) (out []byte, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start(stageName)
		defer st.End()
	}

	comp := archToCompiler[cfg.Arch]
	if comp == "" {
		return out, fmt.Errorf("unknown arch %q", cfg.Arch)
	}

	if cfg.PortagePkg != "" {
		if missing, err := checkDeps(ctx, cfg.PortagePkg); err != nil {
			return out, fmt.Errorf("failed checking deps for %s: %v", cfg.PortagePkg, err)
		} else if len(missing) > 0 {
			b := bytes.NewBufferString("To install missing dependencies, run:\n\n  sudo emerge -j 16 \\\n")
			for i, dep := range missing {
				suffix := ""
				if i < len(missing)-1 {
					suffix = " \\"
				}
				fmt.Fprintf(b, "    =%s%s\n", dep, suffix)
			}
			return b.Bytes(), fmt.Errorf("%s has missing dependencies", cfg.PortagePkg)
		}
	}

	cmd := exec.Command(comp, "install", "-ldflags=-s -w", pkg)
	cmd.Env = append(os.Environ(),
		"GOPATH="+strings.Join([]string{cfg.TestWorkspace, cfg.CommonWorkspace, cfg.SysGopath}, ":"),
		"GOBIN="+outDir,
	)
	if out, err = cmd.CombinedOutput(); err != nil {
		return out, err
	}
	return out, nil
}
