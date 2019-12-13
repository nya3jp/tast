// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
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

	"github.com/google/subcommands"

	"chromiumos/tast/bundle"
	"chromiumos/tast/cmd/tast/build"
	"chromiumos/tast/host"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

const (
	localRunnerPkg  = "chromiumos/tast/cmd/local_test_runner"  // Go package for local_test_runner
	remoteRunnerPkg = "chromiumos/tast/cmd/remote_test_runner" // Go package for remote_test_runner

	localBundlePkgPathPrefix  = "chromiumos/tast/local/bundles"  // Go package path prefix for local test bundles
	remoteBundlePkgPathPrefix = "chromiumos/tast/remote/bundles" // Go package path prefix for remote test bundles

	// localBundleBuildSubdir is a subdirectory used for compiled local test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	localBundleBuildSubdir = "local_bundles"

	// remoteBundleBuildSubdir is a subdirectory used for compiled remote test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	remoteBundleBuildSubdir = "remote_bundles"
)

// Status describes the result of a Run call.
type Status struct {
	// ExitCode contains the exit code that should be used by the tast process.
	ExitCode subcommands.ExitStatus
	// ErrorMsg describes the reason why the run failed.
	ErrorMsg string
	// FailedBeforeRun is true if a failure occurred before trying to run tests,
	// e.g. while compiling tests. If so, the caller shouldn't write a results dir.
	FailedBeforeRun bool
}

// successStatus describes a successful run.
var successStatus = Status{}

// errorStatusf returns a Status describing a failing run. format and args are combined to produce the error
// message, which is both logged to cfg.Logger and included in the returned status.
func errorStatusf(cfg *Config, code subcommands.ExitStatus, format string, args ...interface{}) Status {
	msg := fmt.Sprintf(format, args...)
	cfg.Logger.Log(msg)
	return Status{ExitCode: code, ErrorMsg: msg}
}

// Run executes or lists tests per cfg and returns the results.
// Messages are logged using cfg.Logger as the run progresses.
// If an error is encountered, status.ErrorMsg will be logged to cfg.Logger before returning,
// but the caller may wish to log it again later to increase its prominence if additional messages are logged.
func Run(ctx context.Context, cfg *Config) (status Status, results []TestResult) {
	defer func() {
		// If we didn't get to the point where we started trying to run tests,
		// report that to the caller so they can avoid writing a useless results dir.
		if status.ExitCode == subcommands.ExitFailure && !cfg.startedRun {
			status.FailedBeforeRun = true
		}
	}()

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to connect to %s: %v", cfg.Target, err), nil
	}

	// Start an ephemeral devserver if necessary. Devservers are required in
	// prepare (to download private bundles if -downloadprivatebundles if set)
	// and in local (to download external data files).
	// TODO(crbug.com/982181): Once we move the logic to download external data
	// files to the prepare, try restricting the lifetime of the ephemeral
	// devserver.
	if cfg.runLocal && len(cfg.devservers) == 0 && cfg.useEphemeralDevserver {
		if err := startEphemeralDevserver(ctx, hst, cfg); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to start ephemeral devserver: %v", err), nil
		}
		defer closeEphemeralDevserver(ctx, cfg)
	}

	if err := prepare(ctx, cfg, hst); err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to build and push: %v", err), nil
	}

	// Run local tests.
	if cfg.runLocal {
		status, results = local(ctx, cfg)
	}

	// Turn down the ephemeral devserver before running remote tests. Some remote tests
	// in the meta category run the tast command which starts yet another ephemeral devserver
	// and reverse forwarding port can conflict.
	closeEphemeralDevserver(ctx, cfg)

	// Run remote tests and merge the results.
	// TODO(derat): While test runners are always supposed to report success even if tests fail,
	// it'd probably be better to run both types here even if one fails.
	if cfg.runRemote && status.ExitCode == subcommands.ExitSuccess {
		var rres []TestResult
		status, rres = remote(ctx, cfg)
		results = append(results, rres...)
	}

	return status, results
}

// prepare prepares the DUT for running tests. When instructed in cfg, it builds
// and pushes the local test runner and test bundles, and downloads private test
// bundles.
func prepare(ctx context.Context, cfg *Config, hst *host.SSH) error {
	if cfg.build && cfg.downloadPrivateBundles {
		// Usually it makes no sense to download prebuilt private bundles when
		// building and pushing a fresh test bundle.
		return errors.New("-downloadprivatebundles requires -build=false")
	}

	written := false

	if cfg.build {
		if err := buildAll(ctx, cfg, hst); err != nil {
			return err
		}
		if err := pushAll(ctx, cfg, hst); err != nil {
			return err
		}
		written = true
	}

	if cfg.downloadPrivateBundles {
		if err := downloadPrivateBundles(ctx, cfg, hst); err != nil {
			return fmt.Errorf("failed downloading private bundles: %v", err)
		}
		written = true
	}

	// TODO(crbug.com/982181): Consider downloading external data files here.

	// After writing files to the DUT, run sync to make sure the written files are persisted
	// even if the DUT crashes later. This is important especially when we push local_test_runner
	// because it can appear as zero-byte binary after a crash and subsequent sysinfo phase fails.
	if written {
		if err := hst.Command("sync").Run(ctx); err != nil {
			return fmt.Errorf("failed to sync disk writes: %v", err)
		}
	}
	return nil
}

// buildAll builds Go binaries as instructed in cfg.
func buildAll(ctx context.Context, cfg *Config, hst *host.SSH) error {
	if err := getTargetArch(ctx, cfg, hst); err != nil {
		return fmt.Errorf("failed to get arch for %s: %v", cfg.Target, err)
	}

	// local_test_runner is required even if we are running only remote tests,
	// e.g. to compute software dependencies.
	tgts := []*build.Target{
		{
			Pkg:        localRunnerPkg,
			Arch:       cfg.targetArch,
			Workspaces: cfg.commonWorkspaces(),
			Out:        filepath.Join(cfg.buildOutDir, cfg.targetArch, path.Base(localRunnerPkg)),
		},
	}

	if cfg.runLocal {
		tgts = append(tgts, &build.Target{
			Pkg:        path.Join(localBundlePkgPathPrefix, cfg.buildBundle),
			Arch:       cfg.targetArch,
			Workspaces: cfg.bundleWorkspaces(),
			Out:        filepath.Join(cfg.buildOutDir, cfg.targetArch, localBundleBuildSubdir, cfg.buildBundle),
		})
	}
	if cfg.runRemote {
		tgts = append(tgts, &build.Target{
			Pkg:        remoteRunnerPkg,
			Arch:       build.ArchHost,
			Workspaces: cfg.commonWorkspaces(),
			Out:        cfg.remoteRunner,
		}, &build.Target{
			Pkg:        path.Join(remoteBundlePkgPathPrefix, cfg.buildBundle),
			Arch:       build.ArchHost,
			Workspaces: cfg.bundleWorkspaces(),
			Out:        filepath.Join(cfg.remoteBundleDir, cfg.buildBundle),
		})
	}

	var names []string
	for _, tgt := range tgts {
		names = append(names, path.Base(tgt.Pkg))
	}
	cfg.Logger.Logf("Building %s", strings.Join(names, ", "))
	start := time.Now()
	if err := build.Build(ctx, cfg.buildCfg(), tgts); err != nil {
		return fmt.Errorf("build failed: %v", err)
	}
	cfg.Logger.Logf("Built in %v", time.Now().Sub(start).Round(time.Millisecond))
	return nil
}

// getTargetArch queries hst for its userland architecture if it isn't already known and
// saves it to cfg.targetArch. Note that this can be different from the kernel architecture
// returned by "uname -m" on some boards (e.g. aarch64 kernel with armv7l userland).
// TODO(crbug.com/982184): Get rid of this function.
func getTargetArch(ctx context.Context, cfg *Config, hst *host.SSH) error {
	if cfg.targetArch != "" {
		return nil
	}

	ctx, st := timing.Start(ctx, "get_arch")
	defer st.End()
	cfg.Logger.Debug("Getting architecture from target")

	// Get the userland architecture by inspecting an arbitrary binary on the target.
	out, err := hst.Command("file", "-b", "-L", "/sbin/init").Output(ctx)
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

// pushAll pushes the freshly built local test runner, local test bundle executable
// and local test data files to the DUT if necessary. If cfg.mode is
// ListTestsMode data files are not pushed since they are not needed to build
// a list of tests.
func pushAll(ctx context.Context, cfg *Config, hst *host.SSH) error {
	ctx, st := timing.Start(ctx, "push")
	defer st.End()

	// Push executables first. New test bundle is needed later to get the list of
	// data files to push.
	if err := pushExecutables(ctx, cfg, hst); err != nil {
		return fmt.Errorf("failed to push local executables: %v", err)
	}

	if !cfg.runLocal || cfg.mode == ListTestsMode {
		return nil
	}

	// Only consider tests from the newly-pushed bundle.
	bundleGlob := filepath.Join(cfg.localBundleDir, cfg.buildBundle)

	cfg.Logger.Status("Getting data file list")
	paths, err := getDataFilePaths(ctx, cfg, hst, bundleGlob)
	if err != nil {
		return fmt.Errorf("failed to get data file list: %v", err)
	}
	if len(paths) > 0 {
		cfg.Logger.Status("Pushing data files to target")
		pkg := path.Join(localBundlePkgPathPrefix, cfg.buildBundle)
		destDir := filepath.Join(cfg.localDataDir, pkg)
		if err := pushDataFiles(ctx, cfg, hst, destDir, paths); err != nil {
			return fmt.Errorf("failed to push data files: %v", err)
		}
	}
	return nil
}

// pushExecutables pushes the freshly built local test runner, local test bundle
// executable to the DUT if necessary.
func pushExecutables(ctx context.Context, cfg *Config, hst *host.SSH) error {
	srcDir := filepath.Join(cfg.buildOutDir, cfg.targetArch)

	// local_test_runner is required even if we are running only remote tests,
	// e.g. to compute software dependencies.
	files := map[string]string{
		filepath.Join(srcDir, path.Base(localRunnerPkg)): cfg.localRunner,
	}
	if cfg.runLocal {
		files[filepath.Join(srcDir, localBundleBuildSubdir, cfg.buildBundle)] = filepath.Join(cfg.localBundleDir, cfg.buildBundle)
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
func getDataFilePaths(ctx context.Context, cfg *Config, hst *host.SSH, bundleGlob string) (
	paths []string, err error) {
	ctx, st := timing.Start(ctx, "get_data_paths")
	defer st.End()

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

	var ts []testing.TestCase
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
	ctx, st := timing.Start(ctx, "push_data")
	defer st.End()

	cfg.Logger.Log("Pushing data files to target")

	srcDir := filepath.Join(cfg.buildWorkspace, "src", localBundlePkgPathPrefix, cfg.buildBundle)

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

// downloadPrivateBundles executes local_test_runner on hst to download and unpack
// a private test bundles archive corresponding to the Chrome OS version of hst
// if it has not been done yet.
// An archive contains Go executables of local test bundles and their associated
// internal data files and external data link files. Note that remote test
// bundles are not included in archives.
func downloadPrivateBundles(ctx context.Context, cfg *Config, hst *host.SSH) error {
	ctx, st := timing.Start(ctx, "download_private_bundles")
	defer st.End()

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
