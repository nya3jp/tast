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

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/shutil"
)

// archToEnvs maps from a userland architecture name to the corresponding Go command environment variables that
// should be used for building. An architecture name is usually given by "uname -m", but it can
// be different if the kernel and the userland use different architectures (e.g. aarch64 kernel with
// armv7l userland).
var archToEnvs = map[string][]string{
	ArchHost:  nil,
	"x86_64":  {"GOOS=linux", "GOARCH=amd64"},
	"armv7l":  {"GOOS=linux", "GOARCH=arm", "GOARM=7"},
	"aarch64": {"GOOS=linux", "GOARCH=arm64"},
}

// Build builds executables as dictated by cfg.
// tgts is a list of build targets to build in parallel.
func Build(ctx context.Context, cfg *Config, tgts []*Target) error {
	ctx, st := timing.Start(ctx, "build")
	defer st.End()

	if cfg.TastWorkspace != "" {
		if err := checkSourceCompat(cfg.TastWorkspace); err != nil {
			return fmt.Errorf("tast is too old: %v; please run: ./update_chroot", err)
		}
	}

	if cfg.CheckBuildDeps {
		if missing, cmds, err := checkDeps(ctx, cfg.CheckDepsCachePath); err != nil {
			return fmt.Errorf("failed checking build deps: %v; please run: ./update_chroot", err)
		} else if len(missing) > 0 {
			if !cfg.InstallPortageDeps {
				logMissingDeps(ctx, missing, cmds)
				return errors.New("missing build dependencies")
			}
			if err := installMissingDeps(ctx, missing, cmds); err != nil {
				return fmt.Errorf("failed installing missing deps: %v; please run: ./update_chroot", err)
			}
		}
	}

	// Compile targets in parallel.
	g, ctx := errgroup.WithContext(ctx)
	for _, tgt := range tgts {
		tgt := tgt // bind to iteration-scoped variable
		g.Go(func() error {
			if err := buildOne(ctx, tgt); err != nil {
				return fmt.Errorf("failed to build %s: %v", tgt.Pkg, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// buildOne builds one executable.
func buildOne(ctx context.Context, tgt *Target) error {
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

	archEnvs, ok := archToEnvs[tgt.Arch]
	if !ok {
		return fmt.Errorf("unknown arch %q", tgt.Arch)
	}

	flags := "-ldflags=-s -w"
	if tgt.Debug {
		flags = "-gcflags=all=-N -l"
	}
	cmd := exec.Command("go", "build", flags, "-o", tgt.Out, tgt.Pkg)
	cmd.Env = append(os.Environ(),
		"GOPATH="+strings.Join(tgt.Workspaces, ":"),
		// Disable cgo and PIE on building Tast binaries. See:
		// https://crbug.com/976196
		// https://github.com/golang/go/issues/30986#issuecomment-475626018
		"CGO_ENABLED=0",
		// Tast in Chrome OS is built in GOPATH mode.
		"GO111MODULE=off",
		"GOPIE=0")
	cmd.Env = append(cmd.Env, archEnvs...)

	if out, err := cmd.CombinedOutput(); err != nil {
		writeMultiline(ctx, string(out))
		return err
	}
	return nil
}

// logMissingDeps prints logs describing how to run cmds to install the listed missing packages.
// missing and cmds should be produced by checkDeps.
func logMissingDeps(ctx context.Context, missing []string, cmds [][]string) {
	logging.Info(ctx, "The following dependencies are not installed:")
	for _, dep := range missing {
		logging.Info(ctx, "  ", dep)
	}
	logging.Info(ctx)
	logging.Info(ctx, "To install them, please run the following in your chroot:")
	for _, cmd := range cmds {
		logging.Info(ctx, "  ", shutil.EscapeSlice(cmd))
	}
	logging.Info(ctx)
}

// installMissingDeps attempts to install the supplied missing packages by running cmds in sequence.
// Progress is logged using log. missing and cmds should be produced by checkDeps.
func installMissingDeps(ctx context.Context, missing []string, cmds [][]string) error {
	ctx, st := timing.Start(ctx, "install_deps")
	defer st.End()

	logging.Info(ctx, "Installing missing dependencies:")
	for _, dep := range missing {
		logging.Info(ctx, "  ", dep)
	}
	for _, cmd := range cmds {
		logging.Infof(ctx, "Running %s", shutil.EscapeSlice(cmd))
		if out, err := exec.CommandContext(ctx, cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			logging.Infof(ctx, "Failed running %s", shutil.EscapeSlice(cmd))
			writeMultiline(ctx, string(out))
			return fmt.Errorf("failed running %s: %v", shutil.EscapeSlice(cmd), err)
		}
	}
	return nil
}

// writeMultiline writes multiline text s to log.
func writeMultiline(ctx context.Context, s string) {
	if s == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		logging.Info(ctx, line)
	}
}
