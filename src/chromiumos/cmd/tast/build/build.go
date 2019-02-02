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

	"chromiumos/tast/timing"
)

// GetLocalArch returns the local system's architecture as described by "uname -m".
func GetLocalArch() (string, error) {
	b, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// archToCompiler maps from a userland architecture name to the corresponding Go command that
// should be used for building. An architecture name is usually given by "uname -m", but it can
// be different if the kernel and the userland use different architectures (e.g. aarch64 kernel with
// armv7l userland).
// TODO(derat): What's the right way to get the toolchain name for a given board?
// "cros_setup_toolchains --show-board-cfg <board>" seems to print it, but it's very slow (700+ ms).
var archToCompiler = map[string]string{
	"x86_64":  "x86_64-cros-linux-gnu-go",
	"armv7l":  "armv7a-cros-linux-gnueabihf-go",
	"aarch64": "aarch64-cros-linux-gnu-go",
}

// Build builds executable package pkg to outDir as dictated by cfg.
// The executable file's name is assigned by "go install" (i.e. it's the last component of pkg).
// stageName is used as the name of a new stage reported via the timing package.
func Build(ctx context.Context, cfg *Config, pkg, outDir, stageName string) (out []byte, err error) {
	defer timing.Start(ctx, stageName).End()

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
		if missing, cmds, err := checkDeps(ctx, cfg.PortagePkg, cfg.CheckDepsCachePath); err != nil {
			return out, fmt.Errorf("failed checking deps for %s: %v", cfg.PortagePkg, err)
		} else if len(missing) > 0 {
			// TODO(derat): Consider running these commands automatically instead of printing them
			// if we can confirm that sudo won't prompt for a password.
			b := bytes.NewBufferString("The following dependencies are not installed:\n")
			for _, dep := range missing {
				fmt.Fprintf(b, "  %s\n", dep)
			}
			b.WriteString("\nTo install them, please run the following in your chroot:\n")
			for _, cmd := range cmds {
				fmt.Fprintf(b, "  %s\n", cmd)
			}
			b.WriteString("\n")
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

	defer timing.Start(ctx, "compile").End()
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
