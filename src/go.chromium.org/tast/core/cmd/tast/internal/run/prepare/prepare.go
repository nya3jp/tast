// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package prepare implements the preparation phase of Tast CLI. Preparation
// steps include building/pushing test bundles (when -build=true) and
// downloading private test bundles (when -build=false).
package prepare

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast/core/cmd/tast/internal/build"
	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/internal/debugger"
	"go.chromium.org/tast/core/internal/linuxssh"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/internal/timing"
	"go.chromium.org/tast/core/ssh"

	fwprotocol "go.chromium.org/tast/core/framework/protocol"
)

// CheckPrivateBundleFlag instructed in cfg,
// it builds and pushes the local test runner and test
// bundles, and downloads private test bundles.
func CheckPrivateBundleFlag(ctx context.Context, cfg *config.Config) error {
	if cfg.Build() && cfg.DownloadPrivateBundles() {
		// Usually it makes no sense to download prebuilt private bundles when
		// building and pushing a fresh test bundle.
		return errors.New("-downloadprivatebundles requires -build=false")
	}
	return nil
}

// Prepare prepares target DUT for running tests.
// It returns the DUTInfo for the primary DUT.
func Prepare(ctx context.Context, cfg *config.Config, driver *driver.Driver) (*protocol.DUTInfo, error) {
	// Build all the remote bundles.
	if err := buildRemoteBundles(ctx, cfg); err != nil {
		return nil, err
	}

	// Do not build or push to DUT as we dont have access to it.
	if !config.ShouldConnect(cfg.Target()) {
		return &protocol.DUTInfo{
			Features: &fwprotocol.DUTFeatures{
				Software: &fwprotocol.SoftwareFeatures{},
				Hardware: &fwprotocol.HardwareFeatures{},
			},
		}, nil
	}

	dutInfo, err := prepareDUT(ctx, cfg, driver)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare DUT %s: %v", cfg.Target(), err)
	}

	return dutInfo, nil
}

// prepareDUT prepares the DUT for running tests and downloads private test bundles.
func prepareDUT(ctx context.Context, cfg *config.Config, drv *driver.Driver) (*protocol.DUTInfo, error) {
	if cfg.Build() {
		targetArch, err := getTargetArch(ctx, cfg, drv.SSHConn())
		if err != nil {
			return nil, fmt.Errorf("failed to get architecture information: %v", err)
		}
		if err := buildLocalBundles(ctx, cfg, targetArch); err != nil {
			return nil, err
		}
		if err := pushAll(ctx, cfg, drv, targetArch); err != nil {
			return nil, err
		}
	}

	// Stream log files from the DUT.
	streamLogs(ctx, cfg, drv)

	// Now that local_test_runner is prepared, we can retrieve DUTInfo.
	// It is needed in DownloadPrivateBundles below.
	return getDUTInfo(ctx, cfg, drv)
}

// getDUTInfo downloads private bundles and returns the DUT info.
func getDUTInfo(ctx context.Context, cfg *config.Config, drv *driver.Driver) (*protocol.DUTInfo, error) {
	dutInfo, err := drv.GetDUTInfo(ctx)
	if err != nil {
		return nil, err
	}

	if cfg.DownloadPrivateBundles() {
		if err := drv.DownloadPrivateBundles(ctx, dutInfo); err != nil {
			return nil, fmt.Errorf("failed downloading private bundles: %v", err)
		}
	}

	// TODO(crbug.com/982181): Consider downloading external data files here.

	// After writing files to the DUT, run sync to make sure the written files are persisted
	// even if the DUT crashes later. This is important especially when we push local_test_runner
	// because it can appear as zero-byte binary after a crash and subsequent sysinfo phase fails.
	if err := drv.SSHConn().CommandContext(ctx, "sync").Run(); err != nil {
		return nil, fmt.Errorf("failed to sync disk writes: %v", err)
	}
	return dutInfo, nil
}

func buildRemoteBundles(ctx context.Context, cfg *config.Config) error {
	targets := []*build.Target{
		{
			Pkg:        build.RemoteRunnerPkg,
			Arch:       build.ArchHost,
			Workspaces: cfg.CommonWorkspaces(),
			Out:        cfg.RemoteRunner(),
			Debug:      cfg.DebuggerPorts()[debugger.RemoteTestRunner] != 0,
		},
		{
			Pkg:        path.Join(remoteBundlePrefix(cfg.BuildBundle()), cfg.BuildBundle()),
			Arch:       build.ArchHost,
			Workspaces: cfg.BundleWorkspaces(),
			Out:        filepath.Join(cfg.RemoteBundleDir(), cfg.BuildBundle()),
			Debug:      cfg.DebuggerPorts()[debugger.RemoteBundle] != 0,
		},
	}

	return buildBundles(ctx, cfg, targets)
}

func remoteBundlePrefix(bundle string) string {
	if bundle == "crosint" {
		return build.RemotePrivateBundlePkgPathPrefix
	}
	return build.RemoteBundlePkgPathPrefix
}

func buildLocalBundles(ctx context.Context, cfg *config.Config, targetArch string) error {
	// local_test_runner is required even if we are running only remote tests,
	// e.g. to compute software dependencies.
	targets := []*build.Target{
		{
			Pkg:        build.LocalRunnerPkg,
			Arch:       targetArch,
			Workspaces: cfg.CommonWorkspaces(),
			Out:        filepath.Join(cfg.BuildOutDir(), targetArch, path.Base(build.LocalRunnerPkg)),
			Debug:      cfg.DebuggerPorts()[debugger.LocalTestRunner] != 0,
		},
		{
			Pkg:        path.Join(localBundlePrefix(cfg.BuildBundle()), cfg.BuildBundle()),
			Arch:       targetArch,
			Workspaces: cfg.BundleWorkspaces(),
			Out:        filepath.Join(cfg.BuildOutDir(), targetArch, build.LocalBundleBuildSubdir, cfg.BuildBundle()),
			Debug:      cfg.DebuggerPorts()[debugger.LocalBundle] != 0,
		},
	}

	return buildBundles(ctx, cfg, targets)
}

func localBundlePrefix(bundle string) string {
	if bundle == "crosint" {
		return build.LocalPrivateBundlePkgPathPrefix
	}
	return build.LocalBundlePkgPathPrefix
}

func buildBundles(ctx context.Context, cfg *config.Config, tgts []*build.Target) error {
	// Nothing to build.
	if !cfg.Build() {
		return nil
	}

	var names []string
	for _, tgt := range tgts {
		names = append(names, path.Base(tgt.Pkg))
	}
	logging.Infof(ctx, "Building %s", strings.Join(names, ", "))
	start := time.Now()
	if err := build.Build(ctx, cfg.BuildCfg(), tgts); err != nil {
		return fmt.Errorf("build failed: %v", err)
	}
	logging.Infof(ctx, "Built in %v", time.Now().Sub(start).Round(time.Millisecond))
	return nil
}

// getTargetArch queries hst for its userland architecture and return the result.
// Note that this can be different from the kernel architecture
// returned by "uname -m" on some boards (e.g. aarch64 kernel with armv7l userland).
func getTargetArch(ctx context.Context, cfg *config.Config, hst *ssh.Conn) (targetArch string, err error) {
	ctx, st := timing.Start(ctx, "get_arch")
	defer st.End()
	logging.Debug(ctx, "Getting architecture from target")

	// Get the userland architecture by inspecting an arbitrary binary on the target.
	out, err := hst.CommandContext(ctx, "file", "-b", "-L", "/sbin/init").CombinedOutput()
	if err != nil {
		return targetArch, fmt.Errorf("file command failed: %v (output: %q)", err, string(out))
	}
	s := string(out)

	if strings.Contains(s, "x86-64") {
		targetArch = "x86_64"
	} else {
		if strings.HasPrefix(s, "ELF 64-bit") {
			targetArch = "aarch64"
		} else {
			targetArch = "armv7l"
		}
	}
	return targetArch, nil
}

// pushAll pushes the freshly built local test runner, local test bundle executable
// and local test data files to the DUT if necessary. If cfg.mode is
// ListTestsMode data files are not pushed since they are not needed to build
// a list of tests.
func pushAll(ctx context.Context, cfg *config.Config, drv *driver.Driver, targetArch string) error {
	ctx, st := timing.Start(ctx, "push")
	defer st.End()

	// Push executables first. New test bundle is needed later to get the list of
	// data files to push.
	if err := pushExecutables(ctx, cfg, drv.SSHConn(), targetArch); err != nil {
		return fmt.Errorf("failed to push local executables: %v", err)
	}

	if cfg.Mode() != config.RunTestsMode {
		return nil
	}

	paths, err := getDataFilePaths(ctx, cfg, drv)
	if err != nil {
		return fmt.Errorf("failed to get data file list: %v", err)
	}
	if len(paths) > 0 {
		if err := pushDataFiles(ctx, cfg, drv.SSHConn(), cfg.LocalDataDir(), paths); err != nil {
			return fmt.Errorf("failed to push data files: %v", err)
		}
	}
	return nil
}

// pushExecutables pushes the freshly built local test runner, local test bundle
// executable to the DUT if necessary.
func pushExecutables(ctx context.Context, cfg *config.Config, hst *ssh.Conn, targetArch string) error {
	srcDir := filepath.Join(cfg.BuildOutDir(), targetArch)

	// local_test_runner is required even if we are running only remote tests,
	// e.g. to compute software dependencies.
	files := map[string]string{
		filepath.Join(srcDir, path.Base(build.LocalRunnerPkg)):                 cfg.LocalRunner(),
		filepath.Join(srcDir, build.LocalBundleBuildSubdir, cfg.BuildBundle()): filepath.Join(cfg.LocalBundleDir(), cfg.BuildBundle()),
	}

	ctx, st := timing.Start(ctx, "push_executables")
	defer st.End()

	logging.Info(ctx, "Pushing executables to target")
	start := time.Now()
	bytes, err := linuxssh.PutFiles(ctx, hst, files, linuxssh.DereferenceSymlinks)
	if err != nil {
		return err
	}
	logging.Infof(ctx, "Pushed executables in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

func allNeededFixtures(fixtures, tests []*driver.BundleEntity) []*driver.BundleEntity {
	m := make(map[string]*driver.BundleEntity)
	for _, e := range fixtures {
		m[e.Resolved.GetEntity().GetName()] = e
	}
	var res []*driver.BundleEntity
	var dfs func(string)
	dfs = func(name string) {
		if name == "" {
			return
		}
		e, ok := m[name]
		if !ok {
			return
		}
		res = append(res, e)
		delete(m, name)
		dfs(e.Resolved.GetEntity().GetFixture())
	}
	for _, t := range tests {
		dfs(t.Resolved.GetEntity().GetFixture())
	}
	return res
}

// getDataFilePaths returns the paths to data files needed for running
// cfg.Patterns on hst. The returned paths are relative to the package root,
// e.g. "go.chromium.org/tast-tests/cros/local/bundle/<bundle>/<category>/data/<filename>".
func getDataFilePaths(ctx context.Context, cfg *config.Config, drv *driver.Driver) (paths []string, err error) {
	ctx, st := timing.Start(ctx, "get_data_paths")
	defer st.End()

	logging.Debug(ctx, "Getting data file list from target")

	var entities []*driver.BundleEntity // all entities needed to run tests

	// Add tests to entities.
	// We pass nil DUTInfo here because we don't have a DUTInfo yet. This is
	// okay but suboptimal; we have to push data files for tests to be
	// skipped.
	// TODO(b/192433910): Retrieve DUTInfo in advance and push necessary
	// data files only.
	tests, err := drv.ListMatchedLocalTests(ctx, nil)
	if err != nil {
		return nil, err
	}
	entities = append(entities, tests...)

	// Add fixtures tests use to entities.
	// Note that we don't need to disambiguate fixtures from multiple test
	// bundles since this code path gets called only when -build=true and
	// thus only a single test bundle is considered.
	fixtures, err := drv.ListLocalFixtures(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListLocalFixtures: %v", err)
	}
	entities = append(entities, allNeededFixtures(fixtures, tests)...)

	// Compute data that entities may use.
	seenPaths := make(map[string]struct{})
	for _, e := range entities {
		for _, p := range e.Resolved.GetEntity().GetDependencies().GetDataFiles() {
			full := filepath.Clean(filepath.Join(testing.RelativeDataDir(e.Resolved.GetEntity().GetPackage()), p))
			if _, ok := seenPaths[full]; ok {
				continue
			}
			paths = append(paths, full)
			seenPaths[full] = struct{}{}
		}
	}

	logging.Debugf(ctx, "Got data file list with %v file(s)", len(paths))
	return paths, nil
}

// pushDataFiles copies the listed entity data files to destDir on hst.
// destDir is the data directory for Tast, e.g. "/usr/share/tast/data/local".
// The file paths are relative to the package root, i.e. paths take the form
// "go.chromium.org/tast-tests/cros/local/bundle/cros/<category>/data/<filename>".
// Otherwise, files will be copied from cfg.BuildWorkspace.
func pushDataFiles(ctx context.Context, cfg *config.Config, hst *ssh.Conn, destDir string, paths []string) error {
	ctx, st := timing.Start(ctx, "push_data")
	defer st.End()

	logging.Info(ctx, "Pushing data files to target")

	srcDir := filepath.Join(cfg.BuildWorkspace(), "src")

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
	wsBytes, err := linuxssh.PutFiles(ctx, hst, files, linuxssh.DereferenceSymlinks)
	if err != nil {
		return err
	}
	if len(delPaths) > 0 {
		if err = linuxssh.DeleteTree(ctx, hst, destDir, delPaths); err != nil {
			return err
		}
	}
	logging.Infof(ctx, "Pushed data files in %v (sent %s)",
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

// streamLogs streams a set of predefined log files to help debugging when DUT becomes unrepairable.
func streamLogs(ctx context.Context, cfg *config.Config, drv *driver.Driver) {
	filesToStream := []string{
		"/var/log/recover_duts/recover_duts.log",
	}

	for _, f := range filesToStream {
		src := f
		dest := filepath.Join(cfg.ResDir(), "streamed", f)

		// Since ConnCache is not goroutine-safe, create a new driver for this goroutine.
		dd, err := drv.Duplicate(ctx)
		if err != nil {
			logging.Infof(ctx, "Cannot duplicate driver for streaming file: %v", err)
			return
		}

		go func() {
			defer dd.Close(ctx)
			if err := dd.StreamFile(ctx, src, dest); err != nil {
				logging.Debugf(ctx, "Fail to stream file %s", src)
			}
		}()
	}
}
