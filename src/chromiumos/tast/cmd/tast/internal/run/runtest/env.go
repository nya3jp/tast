// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtest

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	gotesting "testing"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/runtest/internal/fakerunner"
	"chromiumos/tast/internal/bundle/bundletest"
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/runner"
)

const (
	// LocalTestRunnerPath is the path to the fake local test runner
	// available on the SSH server.
	LocalTestRunnerPath = "/fake/mock_local_test_runner"
)

// All paths are relative to rootDir.
const (
	tastDir   = "tast"
	resultDir = "tast/results/latest"
	trunkDir  = "trunk"
	tempDir   = "tmp"

	remoteTestRunnerPath = "mock_remote_test_runner"
)

// Env contains information needed to interact with the testing environment
// set up.
type Env struct {
	rootDir   string
	bundleEnv *bundletest.Env
	logger    *loggingtest.Logger
	state     *config.DeprecatedState
}

// SetUp sets up a testing environment for Tast CLI.
//
// Call this function at the beginning of unit tests to set up various fakes
// commonly needed by Tast CLI, e.g. a fake SSH server and fake test runners.
//
// The environment is cleaned up automatically on the end of the current unit
// test.
func SetUp(t *gotesting.T, opts ...EnvOrDUTOption) *Env {
	cfg := defaultEnvConfig()
	for _, opt := range opts {
		opt.applyToEnvConfig(cfg)
	}

	// Prepare various directories. All directories should be under rootDir
	// so that they are remove after the test.
	rootDir, err := ioutil.TempDir("", "tast.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(rootDir) })

	for _, dir := range []string{
		tastDir,
		resultDir,
		trunkDir,
		tempDir,
	} {
		if err := os.MkdirAll(filepath.Join(rootDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// dutConfigLazy creates a dut config with a handler which can be lazily
	// updated. We need this because otherwise we have a cyclic dependency:
	// (1) As bundletest packages is designed so that returned environment is
	//     immutable, adding extra SSH handlers after SetUp is called is not
	//     possible. (bundles depend on handlers)
	// (2) Fake runners are set up with knowledge about bundles. This is only
	//     known after SetUp is called. (runner depends on bundles)
	// (3) SSH handlers need to be set up with knowledge about the runner.
	//     (handlers depend on runner)
	// Here we are breaking (3) with lazy SSH handlers.
	dutConfigLazy := func() (_ *bundletest.DUTConfig, setHandlers func([]fakesshserver.Handler)) {
		var handlers []fakesshserver.Handler
		return &bundletest.DUTConfig{
				ExtraSSHHandlers: []fakesshserver.Handler{
					func(cmd string) (fakesshserver.Process, bool) {
						for _, h := range handlers {
							p, ok := h(cmd)
							if ok {
								return p, ok
							}
						}
						return nil, false
					},
				},
			}, func(hs []fakesshserver.Handler) {
				handlers = hs
			}
	}

	primaryDUT, primarySetHandlers := dutConfigLazy()
	companionDUTs := make(map[string]*bundletest.DUTConfig)
	companionSetHandlers := make(map[string]func([]fakesshserver.Handler))
	for role := range cfg.CompanionDUTs {
		companionDUTs[role], companionSetHandlers[role] = dutConfigLazy()
	}

	bundleEnv := bundletest.SetUp(t,
		bundletest.WithLocalBundles(cfg.LocalBundles...),
		bundletest.WithRemoteBundles(cfg.RemoteBundles...),
		bundletest.WithPrimaryDUT(primaryDUT),
		bundletest.WithCompanionDUTs(companionDUTs),
	)

	primarySetHandlers(runnerSSHHandlers(bundleEnv.LocalBundleDir(), cfg.PrimaryDUT))
	for role, dcfg := range cfg.CompanionDUTs {
		companionSetHandlers[role](runnerSSHHandlers(bundleEnv.LocalBundleDir(), dcfg))
	}

	// Create a fake remote test runner.
	remoteTestRunner := fakerunner.New(&fakerunner.Config{
		BundleDir: filepath.Join(rootDir, bundleEnv.RemoteBundleDir()),
		StaticConfig: &runner.StaticConfig{
			Type: runner.RemoteRunner,
		},
		OnRunTestsInit: cfg.OnRunRemoteTestsInit,
	})
	lo, err := remoteTestRunner.Install(filepath.Join(rootDir, remoteTestRunnerPath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lo.Close() })

	// Create a DeprecatedState object that is cleaned up automatically.
	state := &config.DeprecatedState{}

	return &Env{
		rootDir:   rootDir,
		bundleEnv: bundleEnv,
		logger:    loggingtest.NewLogger(t, logging.LevelInfo),
		state:     state,
	}
}

func runnerSSHHandlers(localBundleDir string, dcfg *dutConfig) []fakesshserver.Handler {
	hs := fakerunner.New(&fakerunner.Config{
		BundleDir: localBundleDir,
		StaticConfig: &runner.StaticConfig{
			Type: runner.LocalRunner,
		},
		GetDUTInfo:             dcfg.GetDUTInfo,
		GetSysInfoState:        dcfg.GetSysInfoState,
		CollectSysInfo:         dcfg.CollectSysInfo,
		DownloadPrivateBundles: dcfg.DownloadPrivateBundles,
		OnRunTestsInit:         dcfg.OnRunLocalTestsInit,
	}).SSHHandlers(LocalTestRunnerPath)
	hs = append(hs, defaultHandlers(dcfg)...)
	hs = append(hs, dcfg.ExtraSSHHandlers...)
	return hs
}

// Context returns a background context. loggingtest.Logger is attached to the
// context so that logs are routed to unit test logs.
func (e *Env) Context() context.Context {
	return logging.AttachLogger(context.Background(), e.logger)
}

// TempDir returns a directory path where callers can save arbitrary temporary
// files. This directory is cleared on the end of the current unit test.
func (e *Env) TempDir() string { return filepath.Join(e.rootDir, tempDir) }

// Config returns a Config struct with reasonable default values to be used in
// unit tests. Callers can pass non-nil mod to alter values of a returned Config
// struct if needed to customize Tast CLI component behavior.
func (e *Env) Config(mod func(cfg *config.MutableConfig)) *config.Config {
	randomID := rand.Int31()
	cfg := &config.MutableConfig{
		// Run all available tests.
		Mode:     config.RunTestsMode,
		Patterns: []string{"*"},
		// Use prebuilt test bundles.
		Build: false,
		// Enable typical pre-flight checks.
		CheckTestDeps:  true,
		CollectSysInfo: true,
		// This is the only shard.
		TotalShards: 1,
		// Fill info to access fake SSH servers.
		Target:        e.bundleEnv.PrimaryServer(),
		KeyFile:       e.bundleEnv.KeyFile(),
		CompanionDUTs: e.bundleEnv.CompanionDUTs(),
		// Set default directory paths to use.
		ResDir:          filepath.Join(e.rootDir, resultDir),
		TastDir:         filepath.Join(e.rootDir, tastDir),
		TrunkDir:        filepath.Join(e.rootDir, trunkDir),
		LocalRunner:     LocalTestRunnerPath,
		LocalBundleDir:  e.bundleEnv.LocalBundleDir(),
		LocalOutDir:     filepath.Join(e.rootDir, fmt.Sprintf("tmp/out/local.%d", randomID)),
		RemoteRunner:    filepath.Join(e.rootDir, remoteTestRunnerPath),
		RemoteBundleDir: e.bundleEnv.RemoteBundleDir(),
		RemoteOutDir:    filepath.Join(e.rootDir, fmt.Sprintf("tmp/out/remote.%d", randomID)),
		PrimaryBundle:   "bundle",
		// Set the message timeout long enough.
		MsgTimeout: time.Minute,
	}
	if mod != nil {
		mod(cfg)
	}
	return cfg.Freeze()
}

// State returns a State struct to be used in unit tests. It is cleaned up on
// the end of the current unit test.
func (e *Env) State() *config.DeprecatedState {
	return e.state
}

// defaultHandlers returns SSH handlers to be installed to a fake SSH server by
// default.
func defaultHandlers(cfg *dutConfig) []fakesshserver.Handler {
	return []fakesshserver.Handler{
		// Pass-through basic shell commands.
		fakesshserver.ShellHandler("exec mkdir "),
		fakesshserver.ShellHandler("exec tar "),
		fakesshserver.ShellHandler("exec sha1sum "),
		fakesshserver.ShellHandler("exec rm -rf -- "),
		// Simulate boot_id.
		fakesshserver.ExactMatchHandler("exec cat /proc/sys/kernel/random/boot_id", func(_ io.Reader, stdout, stderr io.Writer) int {
			bootID, err := cfg.BootID()
			if err != nil {
				fmt.Fprintf(stderr, "ERROR: %v\n", err)
				return 1
			}
			io.WriteString(stdout, bootID)
			return 0
		}),
		// Pretend that the system is always x86_64.
		fakesshserver.ExactMatchHandler("exec file -b -L /sbin/init", func(_ io.Reader, stdout, stderr io.Writer) int {
			io.WriteString(stdout, "/sbin/init: ELF 64-bit LSB shared object, x86-64, version 1\n")
			return 0
		}),
		// Do nothing on sync(1).
		fakesshserver.ExactMatchHandler("exec sync", func(_ io.Reader, _, _ io.Writer) int {
			return 0
		}),
	}
}
