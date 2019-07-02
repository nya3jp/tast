// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package build provides support for compiling tests and related executables.
package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/shutil"
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
func Build(ctx context.Context, cfg *Config, pkg, outDir, stageName string) error {
	ctx, st1 := timing.Start(ctx, stageName)
	defer st1.End()

	for _, ws := range cfg.Workspaces {
		src := filepath.Join(ws, "src")
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return fmt.Errorf("invalid workspace %q (no src subdir)", ws)
		} else if err != nil {
			return err
		}
	}

	comp := archToCompiler[cfg.Arch]
	if comp == "" {
		return fmt.Errorf("unknown arch %q", cfg.Arch)
	}

	if cfg.CheckBuildDeps {
		cfg.Logger.Status("Checking build dependencies")
		if missing, cmds, err := checkDeps(ctx, cfg.CheckDepsCachePath); err != nil {
			return fmt.Errorf("failed checking build deps: %v", err)
		} else if len(missing) > 0 {
			if !cfg.InstallPortageDeps {
				logMissingDeps(cfg.Logger, missing, cmds)
				return errors.New("missing build dependencies")
			}
			if err := installMissingDeps(ctx, cfg.Logger, missing, cmds); err != nil {
				return err
			}
		}
	}

	const ldFlags = "-ldflags=-s -w"

	env := append(os.Environ(),
		"GOPATH="+strings.Join(cfg.Workspaces, ":"),
		// Disable cgo and PIE on building Tast binaries. See:
		// https://crbug.com/976196
		// https://github.com/golang/go/issues/30986#issuecomment-475626018
		"CGO_ENABLED=0",
		"GOPIE=0")

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
		return fmt.Errorf("failed to get local arch: %v", err)
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

	cfg.Logger.Status("Compiling " + pkg)
	ctx, st2 := timing.Start(ctx, "compile")
	defer st2.End()

	if out, err := cmd.CombinedOutput(); err != nil {
		// The compiler won't be installed if the user has never run setup_board for a board using
		// the target arch. Suggest manually setting up toolchains.
		if strings.HasSuffix(err.Error(), exec.ErrNotFound.Error()) {
			cfg.Logger.Log("To install toolchains for all architectures, please run:")
			cfg.Logger.Log()
			cfg.Logger.Log("  sudo ~/trunk/chromite/bin/cros_setup_toolchains -t sdk")
			return err
		}
		writeMultiline(cfg.Logger, string(out))
		return err
	}
	return nil
}

// logMissingDeps prints logs describing how to run cmds to install the listed missing packages.
// missing and cmds should be produced by checkDeps.
func logMissingDeps(log logging.Logger, missing []string, cmds [][]string) {
	log.Log("The following dependencies are not installed:")
	for _, dep := range missing {
		log.Log("  ", dep)
	}
	log.Log()
	log.Log("To install them, please run the following in your chroot:")
	for _, cmd := range cmds {
		log.Log("  ", shutil.EscapeSlice(cmd))
	}
	log.Log()
}

// installMissingDeps attempts to install the supplied missing packages by running cmds in sequence.
// Progress is logged using log. missing and cmds should be produced by checkDeps.
func installMissingDeps(ctx context.Context, log logging.Logger, missing []string, cmds [][]string) error {
	ctx, st := timing.Start(ctx, "install_deps")
	defer st.End()

	log.Log("Installing missing dependencies:")
	for _, dep := range missing {
		log.Log("  ", dep)
	}
	for _, cmd := range cmds {
		log.Status(fmt.Sprintf("Running %s", shutil.EscapeSlice(cmd)))
		log.Logf("Running %s", shutil.EscapeSlice(cmd))
		if out, err := exec.CommandContext(ctx, cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			log.Logf("Failed running %s", shutil.EscapeSlice(cmd))
			writeMultiline(log, string(out))
			return fmt.Errorf("failed running %s: %v", shutil.EscapeSlice(cmd), err)
		}
	}
	return nil
}

// writeMultiline writes multiline text s to log.
func writeMultiline(log logging.Logger, s string) {
	if s == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		log.Log(line)
	}
}
