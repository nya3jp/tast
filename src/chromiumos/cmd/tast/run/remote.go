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
	remoteRunner              = "remote_test_runner"             // executable that runs test bundles
	remoteBundlePkgPathPrefix = "chromiumos/tast/remote/bundles" // Go package path prefix for test bundles
	remoteBundleDir           = "/usr/libexec/tast/bundles"      // dir where packaged test bundles are installed

	// remoteBundleBuildSubdir is a subdirectory used for compiled remote test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	remoteBundleBuildSubdir = "remote_bundles"
)

// Remote runs remote tests as directed by cfg.
func Remote(ctx context.Context, cfg *Config) subcommands.ExitStatus {
	start := time.Now()
	if cfg.Build && cfg.BuildCfg.Arch == "" {
		var err error
		if cfg.BuildCfg.Arch, err = build.GetLocalArch(); err != nil {
			cfg.Logger.Log("Failed to get local arch: ", err)
			return subcommands.ExitFailure
		}
	}

	bundleGlob := filepath.Join(remoteBundleDir, "*")
	if cfg.Build {
		cfg.Logger.Status("Building test bundle")
		buildStart := time.Now()
		bundleDest := cfg.BuildCfg.OutPath(filepath.Join(remoteBundleBuildSubdir, cfg.BuildBundle))
		pkg := path.Join(remoteBundlePkgPathPrefix, cfg.BuildBundle)
		cfg.Logger.Debugf("Building %s from %s to %s", pkg, cfg.BuildCfg.TestWorkspace, bundleDest)
		if out, err := build.Build(ctx, &cfg.BuildCfg, pkg, bundleDest, "build_bundle"); err != nil {
			cfg.Logger.Logf("Failed building test bundle: %v\n\n%s", err, out)
			return subcommands.ExitFailure
		}
		cfg.Logger.Logf("Built test bundle in %v", time.Now().Sub(buildStart).Round(time.Millisecond))

		// Only run tests from the newly-built bundle.
		bundleGlob = bundleDest
	}

	if err := runRemoteRunner(ctx, bundleGlob, cfg); err != nil {
		cfg.Logger.Log("Failed to run tests: ", err)
		return subcommands.ExitFailure
	}
	cfg.Logger.Logf("Ran test(s) in %v", time.Now().Sub(start).Round(time.Millisecond))
	return subcommands.ExitSuccess
}

// runRemoteRunner runs the remote test runner with bundles matched by bundleGlob
// and reads its output.
func runRemoteRunner(ctx context.Context, bundleGlob string, cfg *Config) error {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("run_tests")
		defer st.End()
	}

	args := []string{"-bundles=" + bundleGlob}

	if cfg.PrintMode != DontPrint {
		args = append(args, "-listtests")
		args = append(args, cfg.Patterns...)
		b, err := exec.Command(remoteRunner, args...).Output()
		if err != nil {
			return err
		}
		return printTests(cfg.PrintDest, b, cfg.PrintMode)
	}

	args = append(args, "-report", "-target="+cfg.Target, "-keypath="+cfg.KeyFile)
	args = append(args, cfg.Patterns...)
	cmd := exec.Command(remoteRunner, args...)

	var err error
	var stdout, stderr io.Reader
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return fmt.Errorf("failed to open stdout: %v", err)
	}
	if stderr, err = cmd.StderrPipe(); err != nil {
		return fmt.Errorf("failed to open stderr: %v", err)
	}
	stderrReader := newFirstLineReader(stderr)

	cfg.Logger.Logf("Starting %q", strings.Join(cmd.Args, " "))
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := readTestOutput(ctx, cfg, stdout, os.Rename); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		ln, _ := stderrReader.getLine(stderrTimeout)
		return fmt.Errorf("%v: %v", err, ln)
	}
	return nil
}
