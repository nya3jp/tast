// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/timing"

	"github.com/google/subcommands"
)

const (
	remoteBundlePkgPathPrefix = "chromiumos/tast/remote/bundles" // Go package path prefix for test bundles

	// remoteBundleBuildSubdir is a subdirectory used for compiled remote test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	remoteBundleBuildSubdir = "remote_bundles"
)

// Remote runs remote tests as directed by cfg and returns the command's exit status.
// If non-nil, the returned results may be passed to WriteResults.
func Remote(ctx context.Context, cfg *Config) (subcommands.ExitStatus, []TestResult) {
	start := time.Now()
	if cfg.Build && cfg.BuildCfg.Arch == "" {
		var err error
		if cfg.BuildCfg.Arch, err = build.GetLocalArch(); err != nil {
			cfg.Logger.Log("Failed to get local arch: ", err)
			return subcommands.ExitFailure, nil
		}
	}

	var bundleGlob, dataDir string
	if cfg.Build {
		cfg.Logger.Status("Building test bundle")
		buildStart := time.Now()
		bundleDest := cfg.BuildCfg.OutPath(filepath.Join(remoteBundleBuildSubdir, cfg.BuildBundle))
		pkg := path.Join(remoteBundlePkgPathPrefix, cfg.BuildBundle)
		cfg.Logger.Debugf("Building %s from %s to %s", pkg, cfg.BuildCfg.TestWorkspace, bundleDest)
		if out, err := build.Build(ctx, &cfg.BuildCfg, pkg, bundleDest, "build_bundle"); err != nil {
			cfg.Logger.Logf("Failed building test bundle: %v\n\n%s", err, out)
			return subcommands.ExitFailure, nil
		}
		cfg.Logger.Logf("Built test bundle in %v", time.Now().Sub(buildStart).Round(time.Millisecond))

		// Only run tests from the newly-built bundle, and get test data from the source tree.
		bundleGlob = bundleDest
		dataDir = filepath.Join(cfg.BuildCfg.TestWorkspace, "src")
	} else {
		bundleGlob = filepath.Join(cfg.remoteBundleDir, "*")
		dataDir = cfg.remoteDataDir
	}

	results, err := runRemoteRunner(ctx, cfg, bundleGlob, dataDir)
	if err != nil {
		cfg.Logger.Log("Failed to run tests: ", err)
		return subcommands.ExitFailure, results
	}
	cfg.Logger.Logf("Ran %v remote test(s) in %v", len(results), time.Now().Sub(start).Round(time.Millisecond))
	return subcommands.ExitSuccess, results
}

// runRemoteRunner runs the remote test runner with bundles matched by bundleGlob
// and reads its output.
func runRemoteRunner(ctx context.Context, cfg *Config, bundleGlob, dataDir string) ([]TestResult, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("run_tests")
		defer st.End()
	}

	args := []string{"-bundles=" + bundleGlob}

	if cfg.PrintMode != DontPrint {
		args = append(args, "-listtests")
		args = append(args, cfg.Patterns...)
		b, err := exec.Command(cfg.remoteRunner, args...).Output()
		if err != nil {
			return nil, err
		}
		return nil, printTests(cfg.PrintDest, b, cfg.PrintMode)
	}

	args = append(args, "-report", "-target="+cfg.Target, "-keyfile="+cfg.KeyFile, "-keydir="+cfg.KeyDir, "-datadir="+dataDir)
	args = append(args, cfg.Patterns...)
	cmd := exec.Command(cfg.remoteRunner, args...)

	var err error
	var stdout, stderr io.Reader
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, fmt.Errorf("failed to open stdout: %v", err)
	}
	if stderr, err = cmd.StderrPipe(); err != nil {
		return nil, fmt.Errorf("failed to open stderr: %v", err)
	}
	stderrReader := newFirstLineReader(stderr)

	cfg.Logger.Logf("Starting %q", strings.Join(cmd.Args, " "))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	results, rerr := readTestOutput(ctx, cfg, stdout, os.Rename)

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := cmd.Wait(); err != nil {
		ln, _ := stderrReader.getLine(stderrTimeout)
		return results, fmt.Errorf("%v: %v", err, ln)
	}
	return results, rerr
}
