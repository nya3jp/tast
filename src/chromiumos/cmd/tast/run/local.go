// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/timing"
	"chromiumos/tast/host"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"

	"github.com/google/subcommands"

	"golang.org/x/crypto/ssh"
)

const (
	sshConnectTimeout time.Duration = 10 * time.Second // timeout for establishing SSH connection to DUT
	sshPingTimeout    time.Duration = 5 * time.Second  // timeout for checking if SSH connection to DUT is open

	localRunnerPath       = "/usr/local/bin/local_test_runner"          // on-device executable that runs test bundles
	localRunnerPkg        = "chromiumos/cmd/local_test_runner"          // Go package for local test runner
	localRunnerPortagePkg = "chromeos-base/tast-local-test-runner-9999" // Portage package for local test runner

	localBundlePkgPathPrefix = "chromiumos/tast/local/bundles"                // Go package path prefix for test bundles
	localBundleBuiltinDir    = "/usr/local/libexec/tast/bundles/local"        // on-device dir with preinstalled test bundles
	localBundlePushDir       = "/usr/local/libexec/tast/bundles/local_pushed" // on-device dir with test bundles pushed by tast command

	localDataBuiltinDir = "/usr/local/share/tast/data/local"        // on-device dir with preinstalled test data
	localDataPushDir    = "/usr/local/share/tast/data/local_pushed" // on-device dir with test data pushed by tast command

	// TODO(derat): Delete these after 20180524: https://crbug.com/809185
	localBundleOldBuiltinDir = "/usr/local/libexec/tast/bundles" // old version of localBundleBuiltinDir
	localDataOldBuiltinDir   = "/usr/local/share/tast/data"      // old version of localDataBuiltinDir

	// localBundleBuildSubdir is a subdirectory used for compiled local test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	localBundleBuildSubdir = "local_bundles"
)

// Local runs local tests as directed by cfg and returns the command's exit status.
// If non-nil, the returned results may be passed to WriteResults.
func Local(ctx context.Context, cfg *Config) (subcommands.ExitStatus, []TestResult) {
	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		cfg.Logger.Logf("Failed to connect to %s: %v", cfg.Target, err)
		return subcommands.ExitFailure, nil
	}

	var bundleGlob, dataDir string
	if cfg.Build {
		if bundleGlob, err = buildAndPushBundle(ctx, cfg, hst); err != nil {
			cfg.Logger.Logf("Failed building or pushing tests: %v", err)
			return subcommands.ExitFailure, nil
		}
		dataDir = localDataPushDir
	} else {
		// TODO(derat): Delete this check after 20180524 and always use the new locations: https://crbug.com/809185
		if _, err := hst.Run(ctx, "test -d "+host.QuoteShellArg(localBundleBuiltinDir)); err == nil {
			bundleGlob = filepath.Join(localBundleBuiltinDir, "*")
			dataDir = localDataBuiltinDir
		} else {
			bundleGlob = filepath.Join(localBundleOldBuiltinDir, "*")
			dataDir = localDataOldBuiltinDir
		}
	}

	getInitialSysInfo(ctx, cfg)

	cfg.Logger.Status("Running tests on target")
	start := time.Now()
	results, err := runLocalRunner(ctx, cfg, hst, bundleGlob, dataDir)
	if err != nil {
		cfg.Logger.Log("Failed to run tests: ", err)
		return subcommands.ExitFailure, results
	}
	cfg.Logger.Logf("Ran %v local test(s) in %v", len(results), time.Now().Sub(start).Round(time.Millisecond))
	return subcommands.ExitSuccess, results
}

// connectToTarget establishes an SSH connection to the target specified in cfg.
// The connection will be cached in cfg and should not be closed by the caller.
// If a connection is already established, it will be returned.
func connectToTarget(ctx context.Context, cfg *Config) (*host.SSH, error) {
	// If we already have a connection, reuse it if it's still open.
	if cfg.hst != nil {
		if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
			return cfg.hst, nil
		} else {
			cfg.hst = nil
		}
	}

	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("connect")
		defer st.End()
	}
	cfg.Logger.Status("Connecting to target")
	cfg.Logger.Logf("Connecting to %s", cfg.Target)

	o := host.SSHOptions{
		ConnectTimeout: sshConnectTimeout,
		KeyFile:        cfg.KeyFile,
		KeyDir:         cfg.KeyDir,
		WarnFunc:       func(s string) { cfg.Logger.Log(s) },
	}
	if err := host.ParseSSHTarget(cfg.Target, &o); err != nil {
		return nil, err
	}

	var err error
	if cfg.hst, err = host.NewSSH(ctx, &o); err != nil {
		return nil, err
	}
	return cfg.hst, nil
}

// buildAndPushBundle builds a local test bundle and pushes it to hst as dictated by cfg.
// If tests are going to be executed (rather than printed), data files are also pushed
// to localDataPushDir. A glob that should be passed to the runner to select the bundle
// is returned. Progress is logged via cfg.Logger, but if a non-nil error is returned
// it should be logged by the caller.
func buildAndPushBundle(ctx context.Context, cfg *Config, hst *host.SSH) (bundleGlob string, err error) {
	cfg.Logger.Status("Building test bundle")
	if cfg.BuildCfg.Arch == "" {
		var err error
		if cfg.BuildCfg.Arch, err = getHostArch(ctx, cfg, hst); err != nil {
			return "", fmt.Errorf("failed to get arch for %s: %v", cfg.Target, err)
		}
	}

	start := time.Now()
	src := cfg.BuildCfg.OutPath(filepath.Join(localBundleBuildSubdir, cfg.BuildBundle))
	pkg := path.Join(localBundlePkgPathPrefix, cfg.BuildBundle)
	cfg.Logger.Logf("Building %s from %s", pkg, cfg.BuildCfg.TestWorkspace)
	if out, err := build.Build(ctx, &cfg.BuildCfg, pkg, src, "build_bundle"); err != nil {
		return "", fmt.Errorf("build failed: %v\n\n%s", err, out)
	}
	cfg.Logger.Logf("Built test bundle in %v", time.Now().Sub(start).Round(time.Millisecond))

	cfg.Logger.Status("Pushing test bundle to target")
	if err := pushBundle(ctx, cfg, hst, src, localBundlePushDir); err != nil {
		return "", fmt.Errorf("failed to push bundle: %v", err)
	}

	// Only run tests from the newly-pushed bundle.
	bundleGlob = filepath.Join(localBundlePushDir, cfg.BuildBundle)

	if cfg.PrintMode == DontPrint {
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
			if err = pushDataFiles(ctx, cfg, hst, localDataPushDir, paths); err != nil {
				return "", fmt.Errorf("failed to push data files: %v", err)
			}
		}
	}

	return bundleGlob, nil
}

// getHostArch queries hst for its architecture.
func getHostArch(ctx context.Context, cfg *Config, hst *host.SSH) (string, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("get_arch")
		defer st.End()
	}
	cfg.Logger.Debug("Getting architecture from target")
	out, err := hst.Run(ctx, "uname -m")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// pushBundle copies the test bundle at src on the local machine to dstDir on hst.
func pushBundle(ctx context.Context, cfg *Config, hst *host.SSH, src, dstDir string) error {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("push_bundle")
		defer st.End()
	}
	cfg.Logger.Logf("Pushing test bundle %s to %s on target", src, dstDir)
	start := time.Now()
	bytes, err := hst.PutTree(ctx, filepath.Dir(src), dstDir, []string{filepath.Base(src)})
	if err != nil {
		return err
	}
	cfg.Logger.Logf("Pushed test bundle in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

// getDataFilePaths returns the paths to data files needed for running cfg.Patterns on hst.
func getDataFilePaths(ctx context.Context, cfg *Config, hst *host.SSH, bundleGlob string) ([]string, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("get_data_paths")
		defer st.End()
	}
	cfg.Logger.Debug("Getting data file list from target")

	handle, err := startLocalRunner(ctx, cfg, hst, &runner.Args{
		Mode:       runner.ListTestsMode,
		BundleGlob: bundleGlob,
		Patterns:   cfg.Patterns,
	})
	if err != nil {
		return nil, err
	}
	defer handle.Close(ctx)

	var ts []testing.Test
	if err = readLocalRunnerOutput(ctx, handle, &ts); err != nil {
		return nil, err
	}

	// Get paths relative to the top-level data directory and remove duplicates.
	paths := make([]string, 0)
	seen := make(map[string]struct{})
	for _, t := range ts {
		if t.Data == nil {
			continue
		}
		for _, p := range t.Data {
			p = filepath.Join(t.DataDir(), p)
			if _, ok := seen[p]; !ok {
				paths = append(paths, p)
				seen[p] = struct{}{}
			}
		}
	}

	cfg.Logger.Debugf("Got data file list with %v file(s)", len(paths))
	return paths, nil
}

// pushDataFiles copies the test data files at paths under bc.TestWorkspace on the local machine
// to destDir on hst.
func pushDataFiles(ctx context.Context, cfg *Config, hst *host.SSH, destDir string, paths []string) error {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("push_data")
		defer st.End()
	}
	cfg.Logger.Log("Pushing data files to target")

	for _, p := range paths {
		fp := filepath.Join(cfg.BuildCfg.TestWorkspace, "src", p)
		if !strings.HasPrefix(filepath.Clean(fp),
			filepath.Join(cfg.BuildCfg.TestWorkspace, "src")+"/") {
			return fmt.Errorf("data file path %q escapes base dir", p)
		}
	}

	start := time.Now()
	bytes, err := hst.PutTree(ctx, filepath.Join(cfg.BuildCfg.TestWorkspace, "src"), destDir, paths)
	if err != nil {
		return err
	}
	cfg.Logger.Logf("Pushed data files in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

// localRunnerExists checks whether the local_test_runner executable is present on hst.
// It returns true if it is, false if it isn't, or an error if one was encountered while checking.
func localRunnerExists(ctx context.Context, hst *host.SSH) (bool, error) {
	cmd := fmt.Sprintf("test -e %s", host.QuoteShellArg(localRunnerPath))
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
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("build_and_push_runner")
		defer st.End()
	}

	bc := cfg.BuildCfg
	if bc.PortagePkg != "" {
		bc.PortagePkg = localRunnerPortagePkg
	}

	src := bc.OutPath(filepath.Base(localRunnerPath))
	cfg.Logger.Debugf("Building %s from %s", localRunnerPkg, bc.CommonWorkspace)
	if out, err := build.Build(ctx, &bc, localRunnerPkg, src, "build_runner"); err != nil {
		return fmt.Errorf("failed to build test runner: %v\n\n%s", err, out)
	}

	cfg.Logger.Debugf("Pushing test runner to %s on target", localRunnerPath)
	start := time.Now()
	bytes, err := hst.PutTree(ctx, filepath.Dir(src), filepath.Dir(localRunnerPath),
		[]string{filepath.Base(src)})
	if err != nil {
		return fmt.Errorf("failed to copy test runner: %v", err)
	}
	cfg.Logger.Logf("Pushed test runner in %v (sent %s)",
		time.Now().Sub(start).Round(time.Millisecond), formatBytes(bytes))
	return nil
}

// startLocalRunner starts local_test_runner on hst and passes args to it.
// The caller is responsible for reading the handle's stdout and closing the handle.
func startLocalRunner(ctx context.Context, cfg *Config, hst *host.SSH, args *runner.Args) (*host.SSHCommandHandle, error) {
	handle, err := hst.Start(ctx, localRunnerPath, host.OpenStdin, host.StdoutAndStderr)
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

// runLocalRunner runs local_test_runner to completion on hst.
// If cfg.PrintMode is DontPrint, tests are executed and their results are returned.
// Otherwise, serialized tests are printed to cfg.PrintDest.
func runLocalRunner(ctx context.Context, cfg *Config, hst *host.SSH, bundleGlob, dataDir string) ([]TestResult, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("run_tests")
		defer st.End()
	}

	args := runner.Args{
		BundleGlob: bundleGlob,
		Patterns:   cfg.Patterns,
		DataDir:    dataDir,
	}

	if cfg.PrintMode == DontPrint {
		args.Mode = runner.RunTestsMode
	} else {
		args.Mode = runner.ListTestsMode
	}

	handle, err := startLocalRunner(ctx, cfg, hst, &args)
	if err != nil {
		return nil, err
	}
	defer handle.Close(ctx)

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(handle.Stderr())

	if cfg.PrintMode != DontPrint {
		if err = printTests(cfg.PrintDest, handle.Stdout(), cfg.PrintMode); err != nil {
			return nil, stderrReader.appendToError(err, stderrTimeout)
		}
		return nil, nil
	}

	crf := func(src, dst string) error { return moveFromHost(ctx, cfg, hst, src, dst) }
	results, rerr := readTestOutput(ctx, cfg, handle.Stdout(), crf)

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := handle.Wait(ctx); err != nil {
		return results, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, rerr
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

// moveFromHost copies the tree rooted at src on hst to dst on the local system.
func moveFromHost(ctx context.Context, cfg *Config, hst *host.SSH, src, dst string) error {
	cfg.Logger.Debugf("Copying %s from host to %s", src, dst)
	if err := hst.GetFile(ctx, src, dst); err != nil {
		return err
	}
	cfg.Logger.Debugf("Cleaning %s on host", src)
	if out, err := hst.Run(ctx, fmt.Sprintf("rm -rf %s", host.QuoteShellArg(src))); err != nil {
		cfg.Logger.Logf("Failed cleaning %s: %v\n%s", src, err, out)
	}
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
