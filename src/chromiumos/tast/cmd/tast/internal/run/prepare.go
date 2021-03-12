// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/cmd/tast/internal/build"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
)

// prepare prepares the DUT for running tests. When instructed in cfg, it builds
// and pushes the local test runner and test bundles, and downloads private test
// bundles.
func prepare(ctx context.Context, cfg *config.Config, state *config.State, conn *target.Conn) error {
	if cfg.Build && cfg.DownloadPrivateBundles {
		// Usually it makes no sense to download prebuilt private bundles when
		// building and pushing a fresh test bundle.
		return errors.New("-downloadprivatebundles requires -build=false")
	}

	written := false

	if cfg.Build {
		if err := buildAll(ctx, cfg, state, conn.SSHConn()); err != nil {
			return err
		}
		if err := pushAll(ctx, cfg, state, conn.SSHConn()); err != nil {
			return err
		}
		written = true
	}

	if cfg.DownloadPrivateBundles {
		if err := downloadPrivateBundles(ctx, cfg, conn); err != nil {
			return fmt.Errorf("failed downloading private bundles: %v", err)
		}
		written = true
	}

	// TODO(crbug.com/982181): Consider downloading external data files here.

	// After writing files to the DUT, run sync to make sure the written files are persisted
	// even if the DUT crashes later. This is important especially when we push local_test_runner
	// because it can appear as zero-byte binary after a crash and subsequent sysinfo phase fails.
	if written {
		if err := conn.SSHConn().Command("sync").Run(ctx); err != nil {
			return fmt.Errorf("failed to sync disk writes: %v", err)
		}
	}
	return nil
}

// buildAll builds Go binaries as instructed in cfg.
func buildAll(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) error {
	if err := getTargetArch(ctx, cfg, state, hst); err != nil {
		return fmt.Errorf("failed to get arch for %s: %v", cfg.Target, err)
	}

	// local_test_runner is required even if we are running only remote tests,
	// e.g. to compute software dependencies.
	tgts := []*build.Target{
		{
			Pkg:        build.LocalRunnerPkg,
			Arch:       state.TargetArch,
			Workspaces: cfg.CommonWorkspaces(),
			Out:        filepath.Join(cfg.BuildOutDir, state.TargetArch, path.Base(build.LocalRunnerPkg)),
		},
	}

	if cfg.RunLocal {
		tgts = append(tgts, &build.Target{
			Pkg:        path.Join(build.LocalBundlePkgPathPrefix, cfg.BuildBundle),
			Arch:       state.TargetArch,
			Workspaces: cfg.BundleWorkspaces(),
			Out:        filepath.Join(cfg.BuildOutDir, state.TargetArch, build.LocalBundleBuildSubdir, cfg.BuildBundle),
		})
	}
	if cfg.RunRemote {
		tgts = append(tgts, &build.Target{
			Pkg:        build.RemoteRunnerPkg,
			Arch:       build.ArchHost,
			Workspaces: cfg.CommonWorkspaces(),
			Out:        cfg.RemoteRunner,
		}, &build.Target{
			Pkg:        path.Join(build.RemoteBundlePkgPathPrefix, cfg.BuildBundle),
			Arch:       build.ArchHost,
			Workspaces: cfg.BundleWorkspaces(),
			Out:        filepath.Join(cfg.RemoteBundleDir, cfg.BuildBundle),
		})
	}

	var names []string
	for _, tgt := range tgts {
		names = append(names, path.Base(tgt.Pkg))
	}
	cfg.Logger.Logf("Building %s", strings.Join(names, ", "))
	start := time.Now()
	if err := build.Build(ctx, cfg.BuildCfg(), tgts); err != nil {
		return fmt.Errorf("build failed: %v", err)
	}
	cfg.Logger.Logf("Built in %v", time.Now().Sub(start).Round(time.Millisecond))
	return nil
}

// getTargetArch queries hst for its userland architecture if it isn't already known and
// saves it to state.TargetArch. Note that this can be different from the kernel architecture
// returned by "uname -m" on some boards (e.g. aarch64 kernel with armv7l userland).
// TODO(crbug.com/982184): Get rid of this function.
func getTargetArch(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) error {
	if state.TargetArch != "" {
		return nil
	}

	ctx, st := timing.Start(ctx, "get_arch")
	defer st.End()
	cfg.Logger.Debug("Getting architecture from target")

	// Get the userland architecture by inspecting an arbitrary binary on the target.
	out, err := hst.Command("file", "-b", "-L", "/sbin/init").CombinedOutput(ctx)
	if err != nil {
		return fmt.Errorf("file command failed: %v (output: %q)", err, string(out))
	}
	s := string(out)

	if strings.Contains(s, "x86-64") {
		state.TargetArch = "x86_64"
	} else {
		if strings.HasPrefix(s, "ELF 64-bit") {
			state.TargetArch = "aarch64"
		} else {
			state.TargetArch = "armv7l"
		}
	}
	return nil
}

// pushAll pushes the freshly built local test runner, local test bundle executable
// and local test data files to the DUT if necessary. If cfg.mode is
// ListTestsMode data files are not pushed since they are not needed to build
// a list of tests.
func pushAll(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) error {
	ctx, st := timing.Start(ctx, "push")
	defer st.End()

	// Push executables first. New test bundle is needed later to get the list of
	// data files to push.
	if err := pushExecutables(ctx, cfg, state, hst); err != nil {
		return fmt.Errorf("failed to push local executables: %v", err)
	}

	if !cfg.RunLocal || cfg.Mode == config.ListTestsMode {
		return nil
	}

	paths, err := getDataFilePaths(ctx, cfg, state, hst)
	if err != nil {
		return fmt.Errorf("failed to get data file list: %v", err)
	}
	if len(paths) > 0 {
		pkg := path.Join(build.LocalBundlePkgPathPrefix, cfg.BuildBundle)
		destDir := filepath.Join(cfg.LocalDataDir, pkg)
		if err := pushDataFiles(ctx, cfg, hst, destDir, paths); err != nil {
			return fmt.Errorf("failed to push data files: %v", err)
		}
	}
	return nil
}

// pushExecutables pushes the freshly built local test runner, local test bundle
// executable to the DUT if necessary.
func pushExecutables(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) error {
	srcDir := filepath.Join(cfg.BuildOutDir, state.TargetArch)

	// local_test_runner is required even if we are running only remote tests,
	// e.g. to compute software dependencies.
	files := map[string]string{
		filepath.Join(srcDir, path.Base(build.LocalRunnerPkg)): cfg.LocalRunner,
	}
	if cfg.RunLocal {
		files[filepath.Join(srcDir, build.LocalBundleBuildSubdir, cfg.BuildBundle)] = filepath.Join(cfg.LocalBundleDir, cfg.BuildBundle)
	}

	ctx, st := timing.Start(ctx, "push_executables")
	defer st.End()

	cfg.Logger.Log("Pushing executables to target")
	start := time.Now()
	bytes, err := pushToHost(ctx, cfg, hst, files)
	if err != nil {
		return err
	}
	cfg.Logger.Logf("Pushed executables in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

// getDataFilePaths returns the paths to data files needed for running cfg.Patterns on hst.
// The returned paths are relative to the test bundle directory, i.e. they take the form "<category>/data/<filename>".
func getDataFilePaths(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) (
	paths []string, err error) {
	ctx, st := timing.Start(ctx, "get_data_paths")
	defer st.End()

	cfg.Logger.Debug("Getting data file list from target")

	ts, err := listLocalTests(ctx, cfg, state, hst)
	if err != nil {
		return nil, err
	}

	bundlePath := path.Join(build.LocalBundlePkgPathPrefix, cfg.BuildBundle)
	seenPaths := make(map[string]struct{})
	for _, t := range ts {
		if t.Data == nil {
			continue
		}

		for _, p := range t.Data {
			// t.DataDir returns the file's path relative to the top data dir, i.e. /usr/share/tast/data/local.
			full := filepath.Clean(filepath.Join(testing.RelativeDataDir(t.Pkg), p))
			if !strings.HasPrefix(full, bundlePath+"/") {
				return nil, fmt.Errorf("data file path %q escapes base dir", full)
			}
			// Get the file's path relative to the bundle dir.
			rel := full[len(bundlePath)+1:]
			if _, ok := seenPaths[rel]; ok {
				continue
			}
			paths = append(paths, rel)
			seenPaths[rel] = struct{}{}
		}
	}

	cfg.Logger.Debugf("Got data file list with %v file(s)", len(paths))
	return paths, nil
}

// pushDataFiles copies the listed test data files to destDir on hst.
// destDir is the data directory for this bundle, e.g. "/usr/share/tast/data/local/chromiumos/tast/local/bundles/cros".
// The file paths are relative to the test bundle dir, i.e. paths take the form "<category>/data/<filename>".
// Otherwise, files will be copied from cfg.BuildWorkspace.
func pushDataFiles(ctx context.Context, cfg *config.Config, hst *ssh.Conn, destDir string, paths []string) error {
	ctx, st := timing.Start(ctx, "push_data")
	defer st.End()

	cfg.Logger.Log("Pushing data files to target")

	srcDir := filepath.Join(cfg.BuildWorkspace, "src", build.LocalBundlePkgPathPrefix, cfg.BuildBundle)

	// All paths are relative to the bundle dir.
	var copyPaths, delPaths, missingPaths []string
	for _, p := range paths {
		lp := p + testing.ExternalLinkSuffix
		if _, err := os.Stat(filepath.Join(srcDir, lp)); err == nil {
			// Push the external link file.
			copyPaths = append(copyPaths, lp)
		} else if _, err := os.Stat(filepath.Join(srcDir, p)); err == nil {
			// Push the internal data file and remove the external link file (if any).
			copyPaths = append(copyPaths, p)
			delPaths = append(delPaths, lp)
		} else {
			missingPaths = append(missingPaths, p)
		}
	}

	if len(missingPaths) > 0 {
		return fmt.Errorf("not found: %v", missingPaths)
	}

	files := make(map[string]string)
	for _, p := range copyPaths {
		files[filepath.Join(srcDir, p)] = filepath.Join(destDir, p)
	}

	start := time.Now()
	wsBytes, err := pushToHost(ctx, cfg, hst, files)
	if err != nil {
		return err
	}
	if len(delPaths) > 0 {
		if err = deleteFromHost(ctx, cfg, hst, destDir, delPaths); err != nil {
			return err
		}
	}
	cfg.Logger.Logf("Pushed data files in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(wsBytes))
	return nil
}

// formatBytes formats bytes as a human-friendly string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float32(bytes)/float32(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float32(bytes)/float32(kb))
	}
	return fmt.Sprintf("%d B", bytes)
}
