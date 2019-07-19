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

	"golang.org/x/sync/errgroup"

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

// Build builds executables as dictated by cfg.
// tgts is a list of build targets to build in parallel.
func Build(ctx context.Context, cfg *Config, tgts []*Target) error {
	ctx, st := timing.Start(ctx, "build")
	defer st.End()

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

	// Compile targets in parallel.
	g, ctx := errgroup.WithContext(ctx)
	for _, tgt := range tgts {
		tgt := tgt // bind to iteration-scoped variable
		g.Go(func() error {
			if err := buildOne(ctx, cfg.Logger, tgt); err != nil {
				return fmt.Errorf("failed to build %s: %v", tgt.Pkg, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// buildOne builds one executable.
func buildOne(ctx context.Context, log logging.Logger, tgt *Target) error {
	ctx, st := timing.Start(ctx, filepath.Base(tgt.Pkg))
	defer st.End()

	for _, ws := range tgt.Workspaces {
		src := filepath.Join(ws, "src")
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return fmt.Errorf("invalid workspace %q (no src subdir)", ws)
		} else if err != nil {
			return err
		}
	}

	comp := archToCompiler[tgt.Arch]
	if comp == "" {
		return fmt.Errorf("unknown arch %q", tgt.Arch)
	}

	const ldFlags = "-ldflags=-s -w"
	dest := filepath.Join(tgt.OutDir, filepath.Base(tgt.Pkg))
	cmd := exec.Command(comp, "build", ldFlags, "-o", dest, tgt.Pkg)
	cmd.Env = append(os.Environ(),
		"GOPATH="+strings.Join(tgt.Workspaces, ":"),
		// Disable cgo and PIE on building Tast binaries. See:
		// https://crbug.com/976196
		// https://github.com/golang/go/issues/30986#issuecomment-475626018
		"CGO_ENABLED=0",
		"GOPIE=0")

	log.Status("Compiling " + tgt.Pkg)

	if out, err := cmd.CombinedOutput(); err != nil {
		// The compiler won't be installed if the user has never run setup_board for a board using
		// the target arch. Suggest manually setting up toolchains.
		if strings.HasSuffix(err.Error(), exec.ErrNotFound.Error()) {
			log.Log("To install toolchains for all architectures, please run:")
			log.Log()
			log.Log("  sudo ~/trunk/chromite/bin/cros_setup_toolchains -t sdk")
			return err
		}
		writeMultiline(log, string(out))
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
