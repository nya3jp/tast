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
	"path/filepath"
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
	"x86_64": "x86_64-cros-linux-gnu-go",
	"armv7l": "armv7a-cros-linux-gnueabi-go",
	// On ARM devices with 64-bit kernels, we still have a 32-bit userspace.
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

	for _, ws := range cfg.Workspaces {
		src := filepath.Join(ws, "src")
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return out, fmt.Errorf("invalid workspace %q (no src subdir)", ws)
		} else if err != nil {
			return out, err
		}
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

	const ldFlags = "-ldflags=-s -w"

	env := append(os.Environ(), "GOPATH="+strings.Join(cfg.Workspaces, ":"))

	// This is frustrating:
	//
	// - "go build" always rebuilds the final executable (but not its dependencies).
	// - "go install" avoids rebuilding the executable when it hasn't changed, so it's faster.
	// - We need to set $GOBIN when using "go install" to avoid installing alongside the source code.
	// - But "go install" refuses to cross-compile if $GOBIN is set (by design; see https://golang.org/issue/11778).
	//
	// So, use "go install" when compiling for the local arch to get faster builds,
	// but fall back to using "go build" when cross-compiling.
	larch, err := GetLocalArch()
	if err != nil {
		return out, fmt.Errorf("failed to get local arch: %v", err)
	}

	var cmd *exec.Cmd
	if larch == cfg.Arch {
		cmd = exec.Command(comp, "install", ldFlags, pkg)
		env = append(env, "GOBIN="+outDir)
	} else {
		dest := filepath.Join(outDir, filepath.Base(pkg))
		cmd = exec.Command(comp, "build", ldFlags, "-o", dest, pkg)
	}
	cmd.Env = env

	if out, err = cmd.CombinedOutput(); err != nil {
		// The compiler won't be installed if the user has never run setup_board for a board using
		// the target arch. Suggest manually setting up toolchains.
		if strings.HasSuffix(err.Error(), exec.ErrNotFound.Error()) {
			msg := "To install toolchains for all architectures, please run:\n\n" +
				"  sudo ~/trunk/chromite/bin/cros_setup_toolchains -t sdk\n"
			return []byte(msg), err
		}
		return out, err
	}
	return out, nil
}
