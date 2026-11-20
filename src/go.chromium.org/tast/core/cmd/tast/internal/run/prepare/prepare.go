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

// Prefetched holds what SetUpRemotePrivateBundle did for the primary DUT, so
// that Prepare can reuse it instead of repeating it.
type Prefetched struct {
	// dutInfo is the DUTInfo retrieved from the DUT.
	dutInfo *protocol.DUTInfo
	// localBundlesErr is the result of downloading private local bundles to
	// the DUT.
	localBundlesErr error
}

// SetUpRemotePrivateBundle prepares target HOST for Private Bundles.
// When it downloads private bundles with a DUT to connect to, it also gets the
// DUTInfo and downloads private local bundles to the DUT concurrently, and
// returns them in Prefetched, including a local download error for Prepare to
// report. Otherwise it returns nil.
func SetUpRemotePrivateBundle(ctx context.Context, cfg *config.Config, driver *driver.Driver) (*Prefetched, error) {
	if driver == nil {
		return nil, errors.New("driver is nil")
	}

	var prefetched *Prefetched
	if !cfg.Build() {
		if cfg.DownloadPrivateBundles() {
			var dutInfo *protocol.DUTInfo
			if config.ShouldConnect(cfg.Target()) {
				logging.Infof(ctx, "Connecting to DUT to get DUT info for remote private bundle...")
				var err error
				dutInfo, err = driver.GetDUTInfo(ctx)
				if err != nil {
					logging.Infof(ctx, "Failed to get DUT info: %v", err)
					return nil, err
				}
			}
			if cfg.BuildArtifactsURLOverride() == "" && dutInfo == nil {
				return nil, errors.New("missing build artifacts URL configuration")
			}

			// Download private local bundles to the DUT while downloading
			// private remote bundles on the host since they are independent.
			// Only the goroutine below uses the driver's ConnCache, which is not
			// goroutine-safe, until it finishes: DownloadPrivateRemoteBundles
			// runs the remote test runner on the host without the connection.
			localCtx, cancelLocal := context.WithCancel(ctx)
			defer cancelLocal()
			var localDone chan error
			if config.ShouldConnect(cfg.Target()) {
				localDone = make(chan error, 1)
				go func() {
					localDone <- driver.DownloadPrivateLocalBundles(localCtx, dutInfo)
				}()
			}
			remoteErr := driver.DownloadPrivateRemoteBundles(ctx, dutInfo)
			if localDone != nil {
				if remoteErr != nil {
					// The run fails anyway, so stop the local download instead
					// of waiting for it to complete.
					cancelLocal()
				}
				prefetched = &Prefetched{dutInfo: dutInfo, localBundlesErr: <-localDone}
			}
			if remoteErr != nil {
				return nil, fmt.Errorf("failed downloading Remote private bundles: %v", remoteErr)
			}
		} else if err := buildAllRemoteBundlesInChroot(ctx, cfg); err != nil {
			return nil, fmt.Errorf("failed to check and build remote bundles: %v", err)
		}
	} else if err := buildRemoteBundles(ctx, cfg); err != nil {
		// Build all the remote bundles.
		return nil, err
	}

	logging.Infof(ctx, "SetUpRemotePrivateBundle completed successfully.")
	return prefetched, nil
}

// Prepare prepares target DUT for running tests.
// If prefetched is non-nil and cfg.Build() is false, it reuses what
// SetUpRemotePrivateBundle did instead of querying the DUT and downloading
// private local bundles again. It returns the DUTInfo for the primary DUT.
func Prepare(ctx context.Context, cfg *config.Config, driver *driver.Driver, prefetched *Prefetched) (
	*protocol.DUTInfo, map[string]string, error) {

	// Do not build or push to DUT as we dont have access to it.
	if !config.ShouldConnect(cfg.Target()) {
		hwFeat := cfg.DefaultHardwareFeatures()
		if hwFeat == nil {
			hwFeat = &fwprotocol.HardwareFeatures{}
		}
		logging.Infof(ctx, "Using hwfeat: %s", hwFeat.String())
		return &protocol.DUTInfo{
			Features: &fwprotocol.DUTFeatures{
				Software: &fwprotocol.SoftwareFeatures{},
				Hardware: hwFeat,
			},
		}, nil, nil
	}

	dutInfo, pushedExcutables, err := prepareDUT(ctx, cfg, driver, prefetched)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to prepare DUT %s: %v", cfg.Target(), err)
	}

	return dutInfo, pushedExcutables, nil
}

// buildAllRemoteBundlesInChroot will build all available remote bundles in
// chroot.
func buildAllRemoteBundlesInChroot(ctx context.Context, cfg *config.Config) error {
	if !config.InChroot() {
		// Do not build in chroot.
		return nil
	}
	// check if it is invoked from tast command line.
	execName, err := os.Executable()
	if err != nil || filepath.Base(execName) != "tast" {
		return nil
	}
	logging.Info(ctx, "Build remote bundles")
	var targets []*build.Target
	type bundleRepos struct {
		bundle string
		repo   string
	}
	remoteBundles := []bundleRepos{
		{bundle: "cros", repo: "tast-tests"},
		{bundle: "crosint", repo: "tast-tests-private"},
		{bundle: "crosint_intel", repo: "tests-partners/partner-intel-private"},
	}
	for _, b := range remoteBundles {
		// If there is no source directory, skip the build.
		src := filepath.Join(cfg.TrunkDir(), "src/platform/", b.repo)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		logging.Infof(ctx, "Adding remote bundle %q to build list", b.bundle)
		workspace := cfg.CommonWorkspaces()
		if b.repo != "tast-tests" {
			workspace = append(workspace,
				filepath.Join(cfg.TrunkDir(), "src/platform/tast-tests"))
		}
		workspace = append(workspace, src)
		targets = append(targets, &build.Target{
			Pkg:        path.Join(remoteBundlePrefix(b.bundle), b.bundle),
			Arch:       build.ArchHost,
			Workspaces: workspace,
			Out:        filepath.Join(cfg.RemoteBundleDir(), b.bundle),
			Debug:      cfg.DebuggerPorts()[debugger.RemoteBundle] != 0,
		},
		)
	}
	if len(targets) == 0 {
		return nil
	}
	return buildBundles(ctx, cfg, targets)

}

// prepareDUT prepares the DUT for running tests and downloads private test bundles.
func prepareDUT(ctx context.Context, cfg *config.Config, drv *driver.Driver, prefetched *Prefetched) (
	*protocol.DUTInfo, map[string]string, error) {
	var pushedExcutables map[string]string
	var dutInfo *protocol.DUTInfo
	if cfg.Build() {
		targetArch, err := getTargetArch(ctx, cfg, drv.SSHConn())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get architecture information: %v", err)
		}
		if err := buildLocalBundles(ctx, cfg, targetArch); err != nil {
			return nil, nil, err
		}
		// pushAll returns DUTInfo retrieved after pushing newly built binaries,
		// or nil if it is not needed yet.
		pushedExcutables, dutInfo, err = pushAll(ctx, cfg, drv, targetArch)
		if err != nil {
			return nil, nil, err
		}
		// Discard anything prefetched before pushing newly built binaries.
		prefetched = nil
	} else if prefetched != nil {
		dutInfo = prefetched.dutInfo
	}

	// Stream log files from the DUT.
	streamLogs(ctx, cfg, drv)

	// Now that local_test_runner is prepared, retrieve DUTInfo if it was not
	// already fetched. It is needed in DownloadPrivateLocalBundles below.
	dutInfo, err := getDUTInfo(ctx, cfg, drv, dutInfo, prefetched)
	if err != nil {
		return nil, nil, err
	}
	return dutInfo, pushedExcutables, nil
}

// getDUTInfo queries the DUT if dutInfo is nil, downloads private local bundles
// if configured (unless prefetched holds the result), and returns the DUT info.
func getDUTInfo(ctx context.Context, cfg *config.Config, drv *driver.Driver, dutInfo *protocol.DUTInfo, prefetched *Prefetched) (*protocol.DUTInfo, error) {
	if dutInfo == nil {
		var err error
		dutInfo, err = drv.GetDUTInfo(ctx)
		if err != nil {
			return nil, err
		}
	}

	if cfg.DownloadPrivateBundles() {
		var err error
		if prefetched != nil {
			err = prefetched.localBundlesErr
		} else {
			err = drv.DownloadPrivateLocalBundles(ctx, dutInfo)
		}
		if err != nil {
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

// buildRemoteBundles builds the necessary binaries for remote execution.
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
	if prefix, err := build.RemoteBundlePrefix(bundle); err == nil {
		return prefix
	}
	return build.RemoteBundlePkgPathPrefix
}

func buildLocalBundles(ctx context.Context, cfg *config.Config, targetArch string) error {
	// Nothing to build.
	if !cfg.Build() {
		return nil
	}

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
	if prefix, err := build.LocalBundlePrefix(bundle); err == nil {
		return prefix
	}
	return build.LocalBundlePkgPathPrefix
}

func buildBundles(ctx context.Context, cfg *config.Config, tgts []*build.Target) error {
	var names []string
	for _, tgt := range tgts {
		names = append(names, path.Base(tgt.Pkg))
	}
	logging.Infof(ctx, "Building %s", strings.Join(names, ", "))
	start := time.Now()
	if err := build.Build(ctx, cfg.BuildCfg(), tgts); err != nil {
		return fmt.Errorf("build failed: %v", err)
	}
	logging.Infof(ctx, "Built in %v", time.Since(start).Round(time.Millisecond))
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
		return targetArch, fmt.Errorf("file command failed: %v (output: %q); please check if the DUT is provisioned with a test image", err, string(out))
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
// and local test data files to the DUT if necessary. If cfg.Mode() is not
// RunTestsMode (e.g. ListTestsMode), data files are not pushed and nil DUTInfo
// is returned. Otherwise it also returns DUTInfo retrieved with the new local
// test runner, which is needed to find the data files to push.
func pushAll(ctx context.Context, cfg *config.Config, drv *driver.Driver, targetArch string) (
	map[string]string, *protocol.DUTInfo, error) {
	ctx, st := timing.Start(ctx, "push")
	defer st.End()

	// Push executables first. New test bundle is needed later to get the list of
	// data files to push.
	pushedExcutables, err := pushExecutables(ctx, cfg, drv.SSHConn(), targetArch)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to push local executables: %v", err)
	}

	if cfg.Mode() != config.RunTestsMode {
		return nil, nil, nil
	}

	dutInfo, err := drv.GetDUTInfo(ctx)
	if err != nil {
		return nil, nil, err
	}
	paths, err := getDataFilePaths(ctx, cfg, drv, dutInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get data file list: %v", err)
	}
	if len(paths) > 0 {
		if err := pushDataFiles(ctx, cfg, drv.SSHConn(), cfg.LocalDataDir(), paths); err != nil {
			return nil, nil, fmt.Errorf("failed to push data files: %v", err)
		}
	}
	return pushedExcutables, dutInfo, nil
}

// pushExecutables pushes the freshly built local test runner, local test bundle
// executable to the DUT if necessary.
func pushExecutables(ctx context.Context, cfg *config.Config, hst *ssh.Conn, targetArch string) (
	map[string]string, error) {
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
		return nil, err
	}
	logging.Infof(ctx, "Pushed executables in %v (sent %s)",
		time.Since(start).Round(time.Millisecond), formatBytes(bytes))
	return files, nil
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
// cfg.Patterns on the DUT described by dutInfo. The returned paths are relative
// to the package root, e.g.
// "go.chromium.org/tast-tests/cros/local/bundle/<bundle>/<category>/data/<filename>".
func getDataFilePaths(ctx context.Context, cfg *config.Config, drv *driver.Driver, dutInfo *protocol.DUTInfo) (paths []string, err error) {
	ctx, st := timing.Start(ctx, "get_data_paths")
	defer st.End()

	logging.Debug(ctx, "Getting data file list from target")

	// List tests with the features used to run them, so that data files of
	// tests to be skipped are not pushed and running tests reuses the list.
	// Companion DUT features are unknown until all DUTs are prepared, so
	// multi-DUT runs list tests without features and push data files of all
	// matched tests.
	var features *protocol.Features
	if len(cfg.CompanionDUTs()) == 0 {
		features = cfg.Features(dutInfo.GetFeatures(), nil)
	}
	tests, fixtures, err := drv.ListMatchedLocalTestsAndFixtures(ctx, features)
	if err != nil {
		return nil, err
	}

	var entities []*driver.BundleEntity // all entities needed to run tests

	// Add non-skipped tests to entities.
	for _, t := range tests {
		if len(t.Resolved.GetSkip().GetReasons()) == 0 {
			entities = append(entities, t)
		}
	}

	// Add fixtures tests use to entities.
	// Note that we don't need to disambiguate fixtures from multiple test
	// bundles since this code path gets called only when -build=true and
	// thus only a single test bundle is considered.
	entities = append(entities, allNeededFixtures(fixtures, entities)...)

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

func dataSrcPath(searchPath []string, dataFilePath string) string {
	for _, sp := range searchPath {
		p := filepath.Join(sp, dataFilePath)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
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

	var searchPaths []string
	for _, s := range cfg.BundleWorkspaces() {
		searchPaths = append(searchPaths, filepath.Join(s, "src"))
	}

	files := make(map[string]string)

	// All paths are relative to the bundle dir.
	var delPaths, missingPaths []string
	for _, p := range paths {
		lp := p + testing.ExternalLinkSuffix
		if srcPath := dataSrcPath(searchPaths, lp); srcPath != "" {
			// Push the external link file.
			files[srcPath] = filepath.Join(destDir, lp)
		} else if srcPath := dataSrcPath(searchPaths, p); srcPath != "" {
			// Push the internal data file and remove the external link file (if any).
			files[srcPath] = filepath.Join(destDir, p)
			delPaths = append(delPaths, lp)
		} else {
			missingPaths = append(missingPaths, p)
		}
	}

	if len(missingPaths) > 0 {
		return fmt.Errorf("not found: %v", missingPaths)
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
		time.Since(start).Round(time.Millisecond), formatBytes(wsBytes))
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
			if dd.SSHConn() == nil {
				return
			}
			conn := dd.SSHConn()
			// Make sure the file exist so that we can stream the file later.
			shellCmd := fmt.Sprintf("mkdir -p %s && touch %s", filepath.Dir(src), src)
			out, err := conn.CommandContext(ctx, "sh", "-c", shellCmd).CombinedOutput()
			if err != nil {
				logging.Infof(ctx, "Will not stream %s because %s", src, string(out))
				return
			}
			if err := dd.StreamFile(ctx, src, dest); err != nil {
				logging.Debugf(ctx, "Fail to stream file %s", src)
			}
		}()
	}
}
