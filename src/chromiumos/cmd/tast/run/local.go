// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/subcommands"
	"golang.org/x/crypto/ssh"

	"chromiumos/cmd/tast/build"
	"chromiumos/tast/bundle"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/host"
	"chromiumos/tast/runner"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

const (
	sshConnectTimeout time.Duration = 10 * time.Second // timeout for establishing SSH connection to DUT
	sshPingTimeout    time.Duration = 5 * time.Second  // timeout for checking if SSH connection to DUT is open
	sshRetryInterval  time.Duration = 5 * time.Second  // minimum time to wait between SSH connection attempts

	localRunnerPath       = "/usr/local/bin/local_test_runner"          // on-device executable that runs test bundles
	localRunnerPkg        = "chromiumos/cmd/local_test_runner"          // Go package for local_test_runner
	localRunnerPortagePkg = "chromeos-base/tast-local-test-runner-9999" // Portage package for local_test_runner

	localBundlePkgPathPrefix = "chromiumos/tast/local/bundles"                // Go package path prefix for test bundles
	localBundleBuiltinDir    = "/usr/local/libexec/tast/bundles/local"        // on-device dir with preinstalled test bundles
	localBundlePushDir       = "/usr/local/libexec/tast/bundles/local_pushed" // on-device dir with test bundles pushed by tast command

	localDataBuiltinDir = "/usr/local/share/tast/data"        // on-device dir with preinstalled test data
	localDataPushDir    = "/usr/local/share/tast/data_pushed" // on-device dir with test data pushed by tast command

	// localBundleBuildSubdir is a subdirectory used for compiled local test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	localBundleBuildSubdir = "local_bundles"

	defaultLocalRunnerWaitTimeout = 10 * time.Second // default timeout for waiting for local_test_runner to exit
	heartbeatInterval             = time.Second      // interval for heartbeat messages
)

// local runs local tests as directed by cfg and returns the command's exit status.
// If non-nil, the returned results may be passed to WriteResults.
func local(ctx context.Context, cfg *Config) (Status, []TestResult) {
	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to connect to %s: %v", cfg.Target, err), nil
	}

	if cfg.forceBuildLocalRunner {
		if err = buildAndPushLocalRunner(ctx, cfg, hst); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed building or pushing runner: %v", err), nil
		}
	}

	var bundleGlob, dataDir string
	if cfg.build {
		if bundleGlob, err = buildAndPushBundle(ctx, cfg, hst); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed building or pushing tests: %v", err), nil
		}
		dataDir = localDataPushDir
	} else {
		bundleGlob = filepath.Join(localBundleBuiltinDir, "*")
		dataDir = localDataBuiltinDir
	}

	if len(cfg.devservers) == 0 && cfg.useEphemeralDevserver {
		if err := startEphemeralDevserver(ctx, hst, cfg); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to start ephemeral devserver: %v", err), nil
		}
		defer closeEphemeralDevserver(ctx, cfg)
	}

	if cfg.downloadPrivateBundles {
		if cfg.build {
			return errorStatusf(cfg, subcommands.ExitFailure, "-downloadprivatebundles requires -build=false"), nil
		}
		if err := downloadPrivateBundles(ctx, cfg, hst); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed downloading private bundles: %v", err), nil
		}
	}

	// After pushing files to the DUT, run sync to make sure the written files are persisted
	// even if the DUT crashes later. This is important especially when we push local_test_runner
	// because it can appear as zero-byte binary after a crash and subsequent sysinfo phase will fail.
	if _, err := hst.Run(ctx, "sync"); err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to sync disk writes: %v", err), nil
	}

	switch cfg.mode {
	case ListTestsMode:
		results, _, err := runLocalRunner(ctx, cfg, hst, cfg.Patterns, bundleGlob, dataDir)
		if err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to list tests: %v", err), results
		}
		return successStatus, results
	case RunTestsMode:
		if err := getSoftwareFeatures(ctx, cfg); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to get DUT software features: %v", err), nil
		}
		if err := getInitialSysInfo(ctx, cfg); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to get initial sysinfo: %v", err), nil
		}
		results, err := runLocalTests(ctx, cfg, hst, bundleGlob, dataDir)
		if err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to run tests: %v", err), results
		}
		return successStatus, results
	default:
		return errorStatusf(cfg, subcommands.ExitFailure, "Unhandled mode %d", cfg.mode), nil
	}
}

// connectToTarget establishes an SSH connection to the target specified in cfg.
// The connection will be cached in cfg and should not be closed by the caller.
// If a connection is already established, it will be returned.
func connectToTarget(ctx context.Context, cfg *Config) (*host.SSH, error) {
	// If we already have a connection, reuse it if it's still open.
	if cfg.hst != nil {
		if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
			return cfg.hst, nil
		}
		cfg.hst = nil
	}

	defer timing.Start(ctx, "connect").End()
	cfg.Logger.Status("Connecting to target")
	cfg.Logger.Logf("Connecting to %s", cfg.Target)

	o := host.SSHOptions{
		ConnectTimeout:       sshConnectTimeout,
		ConnectRetries:       cfg.sshRetries,
		ConnectRetryInterval: sshRetryInterval,
		KeyFile:              cfg.KeyFile,
		KeyDir:               cfg.KeyDir,
		WarnFunc:             func(s string) { cfg.Logger.Log(s) },
	}
	if err := host.ParseSSHTarget(cfg.Target, &o); err != nil {
		return nil, err
	}

	var err error
	if cfg.hst, err = host.NewSSH(ctx, &o); err != nil {
		return nil, err
	}

	if cfg.initBootID == "" {
		if cfg.initBootID, err = readBootID(ctx, cfg.hst); err != nil {
			return nil, err
		}
	}

	return cfg.hst, nil
}

// localBundlePackage returns the Portage package name for the bundle with the given name (e.g. "cros").
func localBundlePackage(name string) string {
	return fmt.Sprintf("chromeos-base/tast-local-tests-%s", name)
}

// buildAndPushBundle builds a local test bundle and pushes it to hst as dictated by cfg.
// If tests are going to be executed (rather than printed), data files are also pushed
// to localDataPushDir. A glob that should be passed to the runner to select the bundle
// is returned. Progress is logged via cfg.Logger, but if a non-nil error is returned
// it should be logged by the caller.
func buildAndPushBundle(ctx context.Context, cfg *Config, hst *host.SSH) (bundleGlob string, err error) {
	cfg.Logger.Status("Building test bundle")
	if err := getTargetArch(ctx, cfg, hst); err != nil {
		return "", fmt.Errorf("failed to get arch for %s: %v", cfg.Target, err)
	}

	start := time.Now()
	bc := cfg.baseBuildCfg()
	bc.Arch = cfg.targetArch
	bc.Workspaces = cfg.bundleWorkspaces()
	if cfg.checkPortageDeps {
		bc.PortagePkg = localBundlePackage(cfg.buildBundle) + "-9999"
	}
	buildDir := filepath.Join(cfg.buildOutDir, cfg.targetArch, localBundleBuildSubdir)
	pkg := path.Join(localBundlePkgPathPrefix, cfg.buildBundle)
	cfg.Logger.Logf("Building %s from %s", pkg, strings.Join(bc.Workspaces, ":"))
	if out, err := build.Build(ctx, &bc, pkg, buildDir, "build_bundle"); err != nil {
		return "", fmt.Errorf("build failed: %v\n\n%s", err, out)
	}
	cfg.Logger.Logf("Built test bundle in %v", time.Now().Sub(start).Round(time.Millisecond))

	cfg.Logger.Status("Pushing test bundle to target")
	if err := pushBundle(ctx, cfg, hst, filepath.Join(buildDir, cfg.buildBundle), localBundlePushDir); err != nil {
		return "", fmt.Errorf("failed to push bundle: %v", err)
	}

	// Only run tests from the newly-pushed bundle.
	bundleGlob = filepath.Join(localBundlePushDir, cfg.buildBundle)

	if cfg.mode == RunTestsMode {
		cfg.Logger.Status("Getting data file list")
		var paths []string
		var err error
		if paths, err = getDataFilePaths(ctx, cfg, hst, bundleGlob); err != nil {
			if exists, existsErr := localRunnerExists(ctx, hst); exists || existsErr != nil {
				if existsErr != nil {
					cfg.Logger.Log("Failed to check for existence of runner: ", err)
				}
				return "", fmt.Errorf("failed to get data file list: %v", err)
			}

			// The runner was missing (maybe this is a non-test device), so build and push it and try again.
			if err = buildAndPushLocalRunner(ctx, cfg, hst); err != nil {
				return "", err
			}
			if paths, err = getDataFilePaths(ctx, cfg, hst, bundleGlob); err != nil {
				return "", fmt.Errorf("failed to get data file list: %v", err)
			}
		}
		if len(paths) > 0 {
			cfg.Logger.Status("Pushing data files to target")
			destDir := filepath.Join(localDataPushDir, pkg)
			if err = pushDataFiles(ctx, cfg, hst, destDir, paths); err != nil {
				return "", fmt.Errorf("failed to push data files: %v", err)
			}
		}
	}

	return bundleGlob, nil
}

// getTargetArch queries hst for its userland architecture if it isn't already known and
// saves it to cfg.targetArch. Note that this can be different from the kernel architecture
// returned by "uname -m" on some boards (e.g. aarch64 kernel with armv7l userland).
func getTargetArch(ctx context.Context, cfg *Config, hst *host.SSH) error {
	if cfg.targetArch != "" {
		return nil
	}

	defer timing.Start(ctx, "get_arch").End()
	cfg.Logger.Debug("Getting architecture from target")

	// Get the userland architecture by inspecting an arbitrary binary on the target.
	out, err := hst.Run(ctx, "file -b -L /sbin/init")
	if err != nil {
		return err
	}
	s := string(out)

	if strings.Contains(s, "x86-64") {
		cfg.targetArch = "x86_64"
	} else {
		if strings.HasPrefix(s, "ELF 64-bit") {
			cfg.targetArch = "aarch64"
		} else {
			cfg.targetArch = "armv7l"
		}
	}
	return nil
}

// pushBundle copies the test bundle at src on the local machine to dstDir on hst.
func pushBundle(ctx context.Context, cfg *Config, hst *host.SSH, src, dstDir string) error {
	defer timing.Start(ctx, "push_bundle").End()
	cfg.Logger.Logf("Pushing test bundle %s to %s on target", src, dstDir)
	start := time.Now()
	bytes, err := pushToHost(ctx, cfg, hst, filepath.Dir(src), dstDir, []string{filepath.Base(src)})
	if err != nil {
		return err
	}
	cfg.Logger.Logf("Pushed test bundle in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

// getDataFilePaths returns the paths to data files needed for running cfg.Patterns on hst.
// The returned paths are relative to the test bundle directory, i.e. they take the form "<category>/data/<filename>".
func getDataFilePaths(ctx context.Context, cfg *Config, hst *host.SSH, bundleGlob string) (
	paths []string, err error) {
	defer timing.Start(ctx, "get_data_paths").End()
	cfg.Logger.Debug("Getting data file list from target")

	handle, err := startLocalRunner(ctx, cfg, hst, &runner.Args{
		Mode: runner.ListTestsMode,
		ListTests: &runner.ListTestsArgs{
			BundleArgs: bundle.ListTestsArgs{Patterns: cfg.Patterns},
			BundleGlob: bundleGlob,
		},
	})
	if err != nil {
		return nil, err
	}
	defer handle.Close(ctx)

	var ts []testing.Test
	if err = readLocalRunnerOutput(ctx, handle, &ts); err != nil {
		return nil, err
	}

	bundlePath := path.Join(localBundlePkgPathPrefix, cfg.buildBundle)
	seenPaths := make(map[string]struct{})
	for _, t := range ts {
		if t.Data == nil {
			continue
		}

		for _, p := range t.Data {
			// t.DataDir returns the file's path relative to the top data dir, i.e. /usr/share/tast/data/local.
			full := filepath.Clean(filepath.Join(t.DataDir(), p))
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
// Otherwise, files will be copied from cfg.buildWorkspace.
func pushDataFiles(ctx context.Context, cfg *Config, hst *host.SSH, destDir string, paths []string) error {
	defer timing.Start(ctx, "push_data").End()
	cfg.Logger.Log("Pushing data files to target")

	srcDir := filepath.Join(cfg.buildWorkspace, "src", localBundlePkgPathPrefix, cfg.buildBundle)

	// All paths are relative to the bundle dir.
	var copyPaths []string
	var delPaths []string
	var missingPaths []string
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

	start := time.Now()
	var err error
	var wsBytes, extBytes int64
	if wsBytes, err = pushToHost(ctx, cfg, hst, srcDir, destDir, copyPaths); err != nil {
		return err
	}
	if len(delPaths) > 0 {
		if err = deleteFromHost(ctx, cfg, hst, destDir, delPaths); err != nil {
			return err
		}
	}
	cfg.Logger.Logf("Pushed data files in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(wsBytes+extBytes))
	return nil
}

// downloadPrivateBundles executes local_test_runner on hst to download private
// test bundles if they are not available yet.
func downloadPrivateBundles(ctx context.Context, cfg *Config, hst *host.SSH) error {
	defer timing.Start(ctx, "download_private_bundles").End()

	handle, err := startLocalRunner(ctx, cfg, hst, &runner.Args{
		Mode: runner.DownloadPrivateBundlesMode,
		DownloadPrivateBundles: &runner.DownloadPrivateBundlesArgs{
			Devservers: cfg.devservers,
		},
	})
	if err != nil {
		return err
	}
	defer handle.Close(ctx)

	var res runner.DownloadPrivateBundlesResult
	if err := readLocalRunnerOutput(ctx, handle, &res); err != nil {
		return err
	}
	for _, msg := range res.Messages {
		cfg.Logger.Log(msg)
	}
	return nil
}

// localRunnerExists checks whether the local_test_runner executable is present on hst.
// It returns true if it is, false if it isn't, or an error if one was encountered while checking.
func localRunnerExists(ctx context.Context, hst *host.SSH) (bool, error) {
	cmd := fmt.Sprintf("test -e %s", shutil.Escape(localRunnerPath))
	if _, err := hst.Run(ctx, cmd); err == nil {
		return true, nil
	} else if ee, ok := err.(*ssh.ExitError); ok && ee.Waitmsg.ExitStatus() == 1 {
		return false, nil
	} else {
		return false, err
	}
}

// buildAndPushLocalRunner builds the local_test_runner executable and pushes it to hst.
func buildAndPushLocalRunner(ctx context.Context, cfg *Config, hst *host.SSH) error {
	if err := getTargetArch(ctx, cfg, hst); err != nil {
		return fmt.Errorf("failed to get arch for %s: %v", cfg.Target, err)
	}

	defer timing.Start(ctx, "build_and_push_runner").End()

	bc := cfg.baseBuildCfg()
	bc.Arch = cfg.targetArch
	bc.Workspaces = cfg.commonWorkspaces()
	if cfg.checkPortageDeps {
		bc.PortagePkg = localRunnerPortagePkg
	}

	buildDir := filepath.Join(cfg.buildOutDir, cfg.targetArch)
	cfg.Logger.Debugf("Building %s from %s", localRunnerPkg, strings.Join(bc.Workspaces, ":"))
	if out, err := build.Build(ctx, &bc, localRunnerPkg, buildDir, "build_runner"); err != nil {
		return fmt.Errorf("failed to build test runner: %v\n\n%s", err, out)
	}

	cfg.Logger.Debugf("Pushing test runner to %s on target", localRunnerPath)
	start := time.Now()
	bytes, err := pushToHost(ctx, cfg, hst, buildDir, filepath.Dir(localRunnerPath),
		[]string{filepath.Base(localRunnerPath)})
	if err != nil {
		return fmt.Errorf("failed to copy test runner: %v", err)
	}
	cfg.Logger.Logf("Pushed test runner in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

// runLocalTests executes tests as described by cfg on hst and returns the results.
// It is only used for RunTestsMode.
func runLocalTests(ctx context.Context, cfg *Config, hst *host.SSH, bundleGlob, dataDir string) ([]TestResult, error) {
	cfg.Logger.Status("Running local tests on target")
	defer timing.Start(ctx, "run_local_tests").End()
	cfg.startedRun = true
	start := time.Now()

	// We may temporarily override this later when restarting testing after a failure.
	origCheckTestDeps := cfg.checkTestDeps
	defer func() { cfg.checkTestDeps = origCheckTestDeps }()

	// Run local_test_runner in a loop so we can try to run the remaining tests on failure.
	var allResults []TestResult
	patterns := cfg.Patterns
	for {
		results, unstarted, err := runLocalRunner(ctx, cfg, hst, patterns, bundleGlob, dataDir)
		allResults = append(allResults, results...)
		if err == nil {
			break
		}

		cfg.Logger.Logf("Test runner failed with %v result(s): %v", len(results), err)

		// If local_test_runner didn't provide a list of remaining tests, give up.
		if unstarted == nil {
			return allResults, err
		}
		// If we know that there are no more tests left to execute, report the overall run as having succeeded.
		// The test that was in progress when the run failed will be reported as having failed.
		if len(unstarted) == 0 {
			break
		}
		// If we don't want to try again, or we'd just be doing the same thing that we did last time, give up.
		if !cfg.continueAfterFailure || reflect.DeepEqual(patterns, unstarted) {
			return allResults, err
		}

		cfg.Logger.Logf("Trying to run %v remaining test(s)", len(unstarted))
		oldHst := hst
		var connErr error
		if hst, connErr = connectToTarget(ctx, cfg); connErr != nil {
			cfg.Logger.Log("Failed reconnecting to target: ", connErr)
			return allResults, err
		}
		// The ephemeral devserver uses the SSH connection to the DUT, so a new devserver needs
		// to be created if a new SSH connection was established.
		if cfg.ephemeralDevserver != nil && hst != oldHst {
			if devErr := startEphemeralDevserver(ctx, hst, cfg); devErr != nil {
				cfg.Logger.Log("Failed restarting ephemeral devserver: ", connErr)
				return allResults, err
			}
		}

		// Explicitly request running the remaining tests.
		// "Auto" dependency checking has different behavior when using attribute expressions vs.
		// globs, so make sure that deps will still be checked if they would've been checked initially.
		patterns = unstarted
		if cfg.checkTestDeps == checkTestDepsAuto {
			cfg.checkTestDeps = checkTestDepsAlways
		}
	}

	elapsed := time.Now().Sub(start)
	cfg.Logger.Logf("Ran %v local test(s) in %v", len(allResults), elapsed.Round(time.Millisecond))
	return allResults, nil
}

// startLocalRunner asynchronously starts local_test_runner on hst and passes args to it.
// args.FillDeprecated() is called first to backfill any deprecated fields for old runners.
// The caller is responsible for reading the handle's stdout and closing the handle.
func startLocalRunner(ctx context.Context, cfg *Config, hst *host.SSH, args *runner.Args) (*host.SSHCommandHandle, error) {
	// Set proxy-related environment variables for local_test_runner so it will use them
	// when accessing network.
	envPrefix := ""
	if cfg.proxy == proxyEnv {
		// Proxy-related variables can be either uppercase or lowercase.
		// See https://golang.org/pkg/net/http/#ProxyFromEnvironment.
		for _, name := range []string{
			"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
			"http_proxy", "https_proxy", "no_proxy",
		} {
			if val := os.Getenv(name); val != "" {
				envPrefix += fmt.Sprintf("%s=%s ", name, shutil.Escape(val))
			}
		}
	}

	args.FillDeprecated()

	handle, err := hst.Start(ctx, envPrefix+localRunnerPath, host.OpenStdin, host.StdoutAndStderr)
	if err != nil {
		return nil, err
	}

	if err = json.NewEncoder(handle.Stdin()).Encode(&args); err != nil {
		handle.Close(ctx)
		return nil, fmt.Errorf("write args: %v", err)
	}
	if err = handle.Stdin().Close(); err != nil {
		handle.Close(ctx)
		return nil, fmt.Errorf("close stdin: %v", err)
	}
	return handle, nil
}

// runLocalRunner synchronously runs local_test_runner to completion on hst.
// The supplied patterns (rather than cfg.Patterns) are passed to the runner.
//
// If cfg.mode is RunTestsMode, tests are executed and the results from started tests
// and the names of tests that should have been started but weren't (in the order in which
// they should've been run) are returned.
//
// If cfg.mode is ListTestsMode, serialized test information is returned via TestResult.Test
// but other fields are left blank and unstarted is empty.
func runLocalRunner(ctx context.Context, cfg *Config, hst *host.SSH, patterns []string, bundleGlob, dataDir string) (
	results []TestResult, unstarted []string, err error) {
	defer timing.Start(ctx, "run_local_test_runner").End()

	var args runner.Args

	switch cfg.mode {
	case RunTestsMode:
		args = runner.Args{
			Mode: runner.RunTestsMode,
			RunTests: &runner.RunTestsArgs{
				BundleArgs: bundle.RunTestsArgs{
					Patterns:          patterns,
					DataDir:           dataDir,
					TestVars:          cfg.testVars,
					WaitUntilReady:    cfg.waitUntilReady,
					HeartbeatInterval: heartbeatInterval,
				},
				BundleGlob: bundleGlob,
				Devservers: cfg.devservers,
			},
		}
		setRunnerTestDepsArgs(cfg, &args)
	case ListTestsMode:
		args = runner.Args{
			Mode: runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{
				BundleArgs: bundle.ListTestsArgs{Patterns: patterns},
				BundleGlob: bundleGlob,
			},
		}
	}

	handle, err := startLocalRunner(ctx, cfg, hst, &args)
	if err != nil {
		return nil, nil, err
	}
	defer handle.Close(ctx)

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(handle.Stderr())

	var rerr error
	switch cfg.mode {
	case ListTestsMode:
		results, rerr = readTestList(handle.Stdout())
	case RunTestsMode:
		crf := func(src, dst string) error { return moveFromHost(ctx, cfg, hst, src, dst) }
		df := func(ctx context.Context) string { return diagnoseLocalRunError(ctx, cfg) }
		results, unstarted, rerr = readTestOutput(ctx, cfg, handle.Stdout(), crf, df)
	}

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	timeout := defaultLocalRunnerWaitTimeout
	if cfg.localRunnerWaitTimeout > 0 {
		timeout = cfg.localRunnerWaitTimeout
	}
	wctx, wcancel := context.WithTimeout(ctx, timeout)
	defer wcancel()
	if err := handle.Wait(wctx); err != nil {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}

// readLocalRunnerOutput unmarshals a single JSON value from handle.Stdout into out.
func readLocalRunnerOutput(ctx context.Context, handle *host.SSHCommandHandle, out interface{}) error {
	// Handle errors returned by Wait() first, as they'll be more useful than generic JSON decode errors.
	stderrReader := newFirstLineReader(handle.Stderr())
	jerr := json.NewDecoder(handle.Stdout()).Decode(out)
	if err := handle.Wait(ctx); err != nil {
		return stderrReader.appendToError(err, stderrTimeout)
	}
	return jerr
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

// startEphemeralDevserver starts an ephemeral devserver serving on hst.
// cfg's ephemeralDevserver and devservers fields are updated.
// If ephemeralDevserver is non-nil, it is closed first.
func startEphemeralDevserver(ctx context.Context, hst *host.SSH, cfg *Config) error {
	closeEphemeralDevserver(ctx, cfg) // ignore errors; this may rely on a now-dead SSH connection

	lis, err := hst.ListenTCP(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: ephemeralDevserverPort})
	if err != nil {
		return fmt.Errorf("failed to reverse-forward a port: %v", err)
	}

	cacheDir := filepath.Join(cfg.tastDir, "devserver", "static")
	es, err := newEphemeralDevserver(lis, cacheDir)
	if err != nil {
		return err
	}

	cfg.ephemeralDevserver = es
	cfg.devservers = []string{fmt.Sprintf("http://%s", lis.Addr())}
	return nil
}

// closeEphemeralDevserver closes and resets cfg.ephemeralDevserver if non-nil.
func closeEphemeralDevserver(ctx context.Context, cfg *Config) error {
	var err error
	if cfg.ephemeralDevserver != nil {
		err = cfg.ephemeralDevserver.Close(ctx)
		cfg.ephemeralDevserver = nil
	}
	return err
}

// diagnoseLocalRunError is used to attempt to diagnose the cause of an error encountered
// while running local tests. It returns a string that can be returned by a diagnoseRunErrorFunc.
func diagnoseLocalRunError(ctx context.Context, cfg *Config) string {
	if cfg.hst == nil || ctxutil.DeadlineBefore(ctx, time.Now().Add(sshPingTimeout)) {
		return ""
	}
	if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
		return ""
	}
	return "Lost SSH connection: " + diagnoseSSHDrop(ctx, cfg)
}
