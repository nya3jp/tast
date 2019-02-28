// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/subcommands"

	"chromiumos/cmd/tast/build"
	"chromiumos/tast/bundle"
	"chromiumos/tast/runner"
	"chromiumos/tast/timing"
)

const (
	remoteBundlePkgPathPrefix = "chromiumos/tast/remote/bundles" // Go package path prefix for test bundles

	// remoteBundleBuildSubdir is a subdirectory used for compiled remote test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	remoteBundleBuildSubdir = "remote_bundles"
)

// remote runs remote tests as directed by cfg and returns the command's exit status.
// If non-nil, the returned results may be passed to WriteResults.
func remote(ctx context.Context, cfg *Config) (Status, []TestResult) {
	start := time.Now()

	var bundleGlob, dataDir string
	if cfg.build {
		cfg.Logger.Status("Building test bundle")
		buildStart := time.Now()

		bc := cfg.baseBuildCfg()
		bc.Workspaces = cfg.bundleWorkspaces()
		var err error
		if bc.Arch, err = build.GetLocalArch(); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to get local arch: %v", err), nil
		}
		if cfg.checkPortageDeps {
			bc.PortagePkg = fmt.Sprintf("chromeos-base/tast-remote-tests-%s-9999", cfg.buildBundle)
		}
		buildDir := filepath.Join(cfg.buildOutDir, bc.Arch, remoteBundleBuildSubdir)
		pkg := path.Join(remoteBundlePkgPathPrefix, cfg.buildBundle)
		cfg.Logger.Debugf("Building %s from %s to %s", pkg, strings.Join(bc.Workspaces, ":"), buildDir)
		if out, err := build.Build(ctx, &bc, pkg, buildDir, "build_bundle"); err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed building test bundle: %v\n\n%s", err, out), nil
		}
		cfg.Logger.Logf("Built test bundle in %v", time.Now().Sub(buildStart).Round(time.Millisecond))

		// Only run tests from the newly-built bundle, and get test data from the source tree.
		bundleGlob = filepath.Join(buildDir, cfg.buildBundle)
		dataDir = filepath.Join(cfg.buildWorkspace, "src")
	} else {
		bundleGlob = filepath.Join(cfg.remoteBundleDir, "*")
		dataDir = cfg.remoteDataDir
	}

	if err := getSoftwareFeatures(ctx, cfg); err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to get DUT software features: %v", err), nil
	}
	getInitialSysInfo(ctx, cfg)

	cfg.startedRun = true
	results, err := runRemoteRunner(ctx, cfg, bundleGlob, dataDir)
	if err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to run tests: %v", err), results
	}
	cfg.Logger.Logf("Ran %v remote test(s) in %v", len(results), time.Now().Sub(start).Round(time.Millisecond))
	return successStatus, results
}

// runRemoteRunner runs the remote test runner with bundles matched by bundleGlob
// and reads its output.
func runRemoteRunner(ctx context.Context, cfg *Config, bundleGlob, dataDir string) ([]TestResult, error) {
	defer timing.Start(ctx, "run_remote_tests").End()

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	args := runner.Args{
		BundleGlob: bundleGlob,
		Patterns:   cfg.Patterns,
		DataDir:    dataDir,
		RemoteArgs: runner.RemoteArgs{
			RemoteArgs: bundle.RemoteArgs{
				Target:   cfg.Target,
				KeyFile:  cfg.KeyFile,
				KeyDir:   cfg.KeyDir,
				TastPath: exe,
				RunFlags: []string{
					"-keyfile=" + cfg.KeyFile,
					"-keydir=" + cfg.KeyDir,
					"-remoterunner=" + cfg.remoteRunner,
					"-remotebundledir=" + cfg.remoteBundleDir,
					"-remotedatadir=" + cfg.remoteDataDir,
				},
			},
		},
	}
	switch cfg.mode {
	case RunTestsMode:
		args.Mode = runner.RunTestsMode
		setRunnerTestDepsArgs(cfg, &args)

		// Create an output directory within the results dir so we can just move
		// it to its final destination later.
		outDir, err := ioutil.TempDir(cfg.ResDir, "out.")
		if err != nil {
			return nil, fmt.Errorf("failed to create output dir: %v", err)
		}
		args.OutDir = outDir
	case ListTestsMode:
		args.Mode = runner.ListTestsMode
	}

	cmd := exec.Command(cfg.remoteRunner)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var stdout, stderr io.Reader
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	if stderr, err = cmd.StderrPipe(); err != nil {
		return nil, err
	}
	stderrReader := newFirstLineReader(stderr)

	cfg.Logger.Logf("Starting %v locally", cmd.Path)
	if err = cmd.Start(); err != nil {
		return nil, err
	}

	if err = json.NewEncoder(stdin).Encode(&args); err != nil {
		return nil, fmt.Errorf("write to stdin: %v", err)
	}
	if err = stdin.Close(); err != nil {
		return nil, fmt.Errorf("close stdin: %v", err)
	}

	var results []TestResult
	var rerr error
	switch cfg.mode {
	case ListTestsMode:
		results, rerr = readTestList(stdout)
	case RunTestsMode:
		results, rerr = readTestOutput(ctx, cfg, stdout, os.Rename)
	}

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := cmd.Wait(); err != nil {
		return results, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, rerr
}
