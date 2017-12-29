// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/timing"
	"chromiumos/tast/host"

	"github.com/google/subcommands"
)

const (
	sshConnectTimeout time.Duration = 10 * time.Second // timeout for establishing SSH connection to DUT

	localTestsPackage = "chromiumos/cmd/local_tests" // executable package containing local tests

	localTestsBuiltinPath = "/usr/local/bin/local_tests"        // test executable when installed as part of system image
	localTestsPushPath    = "/usr/local/bin/local_tests_pushed" // test executable when pushed by tast command

	localDataBuiltinDir = "/usr/local/share/tast/data"        // local data dir when installed as part of system image
	localDataPushDir    = "/usr/local/share/tast/data_pushed" // local data dir when pushed by tast command
)

// Local runs local tests as directed by cfg.
func Local(ctx context.Context, cfg *Config) subcommands.ExitStatus {
	cfg.Logger.Status("Connecting to target")
	cfg.Logger.Debugf("Connecting to %s", cfg.Target)
	hst, err := connectToTarget(ctx, cfg.Target, cfg.KeyFile)
	if err != nil {
		cfg.Logger.Logf("Failed to connect to %s: %v", cfg.Target, err)
		return subcommands.ExitFailure
	}
	defer hst.Close(ctx)

	var bin, dataDir string
	if cfg.Build {
		if err = buildAndPushTests(ctx, cfg, hst); err != nil {
			cfg.Logger.Logf("Failed deploying tests: %v", err)
			return subcommands.ExitFailure
		}
		bin = localTestsPushPath
		dataDir = localDataPushDir
	} else {
		bin = localTestsBuiltinPath
		dataDir = localDataBuiltinDir
	}

	cfg.Logger.Status("Running tests on target")
	start := time.Now()
	if err = runLocalTestBinary(ctx, hst, bin, dataDir, cfg); err != nil {
		cfg.Logger.Log("Failed running tests: ", err)
		return subcommands.ExitFailure
	}
	cfg.Logger.Logf("Ran test(s) in %0.1f sec", time.Now().Sub(start).Seconds())
	return subcommands.ExitSuccess
}

// connectToTarget establishes an SSH connection to target using the private key at keyFile.
func connectToTarget(ctx context.Context, target, keyFile string) (*host.SSH, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("connect")
		defer st.End()
	}

	o := host.SSHOptions{}
	if err := host.ParseSSHTarget(target, &o); err != nil {
		return nil, err
	}
	o.ConnectTimeout = sshConnectTimeout
	o.KeyPath = keyFile

	hst, err := host.NewSSH(ctx, &o)
	if err != nil {
		return nil, err
	}
	return hst, nil
}

// buildAndPushTests builds local tests and pushes them to hst as dictated by cfg.
// If tests are going to be executed (rather than printed), data files are also pushed.
// Progress is logged via cfg.Logger, but if a non-nil error is returned it should be
// logged by the caller.
func buildAndPushTests(ctx context.Context, cfg *Config, hst *host.SSH) error {
	cfg.Logger.Status("Building tests")
	if cfg.BuildCfg.Arch == "" {
		var err error
		if cfg.BuildCfg.Arch, err = getHostArch(ctx, hst); err != nil {
			return fmt.Errorf("failed to get arch for %s: %v", cfg.Target, err)
		}
	}

	start := time.Now()
	src := cfg.BuildCfg.OutPath(filepath.Base(localTestsPushPath))
	cfg.Logger.Debugf("Building %s from %s to %s", localTestsPackage, cfg.BuildCfg.TestWorkspace, src)
	if out, err := build.BuildTests(ctx, &cfg.BuildCfg, localTestsPackage, src); err != nil {
		return fmt.Errorf("build failed: %v\n\n%s", err, out)
	}
	cfg.Logger.Logf("Built tests in %0.1f sec", time.Now().Sub(start).Seconds())

	cfg.Logger.Status("Pushing tests to target")
	cfg.Logger.Logf("Pushing tests to %s on target", localTestsPushPath)
	start = time.Now()
	if bytes, err := pushTestBinary(ctx, hst, src, filepath.Dir(localTestsPushPath)); err != nil {
		return fmt.Errorf("pushing tests failed: %v", err)
	} else {
		cfg.Logger.Logf("Pushed tests in %0.1f sec (sent %s)",
			time.Now().Sub(start).Seconds(), formatBytes(bytes))
	}

	if cfg.PrintMode == DontPrint {
		cfg.Logger.Status("Getting data file list")
		cfg.Logger.Log("Getting data file list from ", cfg.Target)
		dp, err := getDataFilePaths(ctx, hst, localTestsPushPath, cfg)
		if err != nil {
			return fmt.Errorf("failed to get data file list: %v", err)
		}
		cfg.Logger.Log("Got data file list")

		cfg.Logger.Status("Pushing data files to target")
		cfg.Logger.Log("Pushing data files to ", cfg.Target)
		start = time.Now()
		if bytes, err := pushDataFiles(ctx, hst, localDataPushDir, dp, &cfg.BuildCfg); err != nil {
			return fmt.Errorf("pushing data files failed: %v", err)
		} else {
			cfg.Logger.Logf("Pushed data files in %0.1f sec (sent %s)",
				time.Now().Sub(start).Seconds(), formatBytes(bytes))
		}
	}

	return nil
}

// getHostArch queries hst for its architecture.
func getHostArch(ctx context.Context, hst *host.SSH) (string, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("get_arch")
		defer st.End()
	}
	out, err := hst.Run(ctx, "uname -m")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// pushTestBinary copies the test binary at src on the local machine to dstDir on hst.
func pushTestBinary(ctx context.Context, hst *host.SSH, src, dstDir string) (bytes int64, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("push_tests")
		defer st.End()
	}
	return hst.PutTree(ctx, filepath.Dir(src), dstDir, []string{filepath.Base(src)})
}

// getLocalTestCommand returns a command for running the test executable bin with
// flags and wildcard patterns pat.
func getLocalTestCommand(bin string, flags, pats []string) string {
	ps := ""
	for _, p := range pats {
		ps += " " + host.QuoteShellArg(p)
	}
	return fmt.Sprintf("%s %s%s", bin, strings.Join(flags, " "), ps)
}

// getDataFilePaths returns the paths to data files needed for running cfg.Patterns on hst.
func getDataFilePaths(ctx context.Context, hst *host.SSH, bin string, cfg *Config) ([]string, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("get_data_paths")
		defer st.End()
	}

	cmd := getLocalTestCommand(bin, []string{"-listdata"}, cfg.Patterns)

	handle, err := hst.Start(ctx, cmd, host.CloseStdin, host.StdoutAndStderr)
	if err != nil {
		return nil, err
	}
	defer handle.Close(ctx)

	stderrReader := newFirstLineReader(handle.Stderr())
	out, _ := ioutil.ReadAll(handle.Stdout()) // Wait() also reports output errors.
	if err = handle.Wait(ctx); err != nil {
		ln, _ := stderrReader.getLine(stderrTimeout)
		return nil, fmt.Errorf("%v: %s", err, ln)
	}

	files := make([]string, 0)
	err = json.Unmarshal(out, &files)
	return files, err
}

// pushDataFiles copies the test data files at paths under bc.TestWorkspace on the local machine
// to destDir on hst.
func pushDataFiles(ctx context.Context, hst *host.SSH, destDir string,
	paths []string, bc *build.Config) (bytes int64, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("push_data")
		defer st.End()
	}

	for _, p := range paths {
		fp := filepath.Join(bc.TestWorkspace, "src", p)
		if !strings.HasPrefix(filepath.Clean(fp), filepath.Join(bc.TestWorkspace, "src")+"/") {
			return 0, fmt.Errorf("data file path %q escapes base dir", p)
		}
	}
	return hst.PutTree(ctx, filepath.Join(bc.TestWorkspace, "src"), destDir, paths)
}

// runLocalTestBinary runs the test binary at bin on hst using cfg.
func runLocalTestBinary(ctx context.Context, hst *host.SSH, bin, dataDir string, cfg *Config) error {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("run_tests")
		defer st.End()
	}

	ps := ""
	for _, p := range cfg.Patterns {
		ps += " " + host.QuoteShellArg(p)
	}

	if cfg.PrintMode != DontPrint {
		cmd := getLocalTestCommand(bin, []string{"-listtests"}, cfg.Patterns)
		b, err := hst.Run(ctx, cmd)
		if err != nil {
			return err
		}
		return printTests(cfg.PrintDest, b, cfg.PrintMode)
	}

	cmd := getLocalTestCommand(bin, []string{"-report", "-datadir=" + dataDir}, cfg.Patterns)
	cfg.Logger.Logf("Starting %q on remote host", cmd)
	handle, err := hst.Start(ctx, cmd, host.CloseStdin, host.StdoutAndStderr)
	if err != nil {
		return err
	}
	defer handle.Close(ctx)

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(handle.Stderr())

	crf := func(src, dst string) error {
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
	if err = readTestOutput(ctx, cfg, handle.Stdout(), crf); err != nil {
		return err
	}

	if err := handle.Wait(ctx); err != nil {
		ln, _ := stderrReader.getLine(stderrTimeout)
		return fmt.Errorf("%v: %v", err, ln)
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
