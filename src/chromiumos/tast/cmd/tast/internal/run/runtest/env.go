// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtest

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	gotesting "testing"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/runtest/internal/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/runtest/internal/fakesshserver"
	"chromiumos/tast/internal/bundle/fakebundle"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/sshtest"
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

	localBundleDir  = "bundles/local"
	remoteBundleDir = "bundles/remote"

	keyFile = "id_rsa"
)

// Env contains information needed to interact with the testing environment
// set up.
type Env struct {
	logger           *loggingtest.Logger
	rootDir          string
	primaryServer    *fakesshserver.Server
	companionServers map[string]*fakesshserver.Server
	state            *config.State
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
		localBundleDir,
		remoteBundleDir,
	} {
		if err := os.MkdirAll(filepath.Join(rootDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Install fake bundle executables.
	fakebundle.InstallAt(t, filepath.Join(rootDir, localBundleDir), cfg.LocalBundles...)
	fakebundle.InstallAt(t, filepath.Join(rootDir, remoteBundleDir), cfg.RemoteBundles...)

	// Create a fake remote test runner.
	remoteTestRunner := fakerunner.New(&fakerunner.Config{
		BundleDir: filepath.Join(rootDir, remoteBundleDir),
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

	// Generate an SSH key pair.
	userKey, hostKey := sshtest.MustGenerateKeys()
	keyData := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(userKey),
	})
	if err := ioutil.WriteFile(filepath.Join(rootDir, keyFile), keyData, 0600); err != nil {
		t.Fatal(err)
	}

	// Start fake SSH servers.
	primaryServer, err := startServer(rootDir, cfg.PrimaryDUT, &userKey.PublicKey, hostKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primaryServer.Stop() })

	companionServers := make(map[string]*fakesshserver.Server)
	for role, dcfg := range cfg.CompanionDUTs {
		server, err := startServer(rootDir, dcfg, &userKey.PublicKey, hostKey)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { server.Stop() })
		companionServers[role] = server
	}

	// Create a State object that is cleaned up automatically.
	state := &config.State{}
	t.Cleanup(func() { state.Close(context.Background()) })

	return &Env{
		logger:           loggingtest.NewLogger(t, logging.LevelInfo),
		rootDir:          rootDir,
		primaryServer:    primaryServer,
		companionServers: companionServers,
		state:            state,
	}
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
	companionDUTs := make(map[string]string)
	for role, server := range e.companionServers {
		companionDUTs[role] = server.Addr().String()
	}
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
		Target:        e.primaryServer.Addr().String(),
		KeyFile:       filepath.Join(e.rootDir, keyFile),
		CompanionDUTs: companionDUTs,
		// Set default directory paths to use.
		ResDir:          filepath.Join(e.rootDir, resultDir),
		TastDir:         filepath.Join(e.rootDir, tastDir),
		TrunkDir:        filepath.Join(e.rootDir, trunkDir),
		LocalRunner:     LocalTestRunnerPath,
		LocalBundleDir:  filepath.Join(e.rootDir, localBundleDir),
		LocalOutDir:     filepath.Join(e.rootDir, fmt.Sprintf("tmp/out/local.%d", randomID)),
		RemoteRunner:    filepath.Join(e.rootDir, remoteTestRunnerPath),
		RemoteBundleDir: filepath.Join(e.rootDir, remoteBundleDir),
		RemoteOutDir:    filepath.Join(e.rootDir, fmt.Sprintf("tmp/out/remote.%d", randomID)),
		PrimaryBundle:   "bundle",
	}
	if mod != nil {
		mod(cfg)
	}
	return cfg.Freeze()
}

// State returns a State struct to be used in unit tests. It is cleaned up on
// the end of the current unit test.
func (e *Env) State() *config.State {
	return e.state
}

// startServer starts a fake SSH server with a fake local test runner installed.
func startServer(rootDir string, cfg *dutConfig, userKey *rsa.PublicKey, hostKey *rsa.PrivateKey) (*fakesshserver.Server, error) {
	runner := fakerunner.New(&fakerunner.Config{
		BundleDir: filepath.Join(rootDir, localBundleDir),
		StaticConfig: &runner.StaticConfig{
			Type: runner.LocalRunner,
		},
		GetDUTInfo:             cfg.GetDUTInfo,
		GetSysInfoState:        cfg.GetSysInfoState,
		CollectSysInfo:         cfg.CollectSysInfo,
		DownloadPrivateBundles: cfg.DownloadPrivateBundles,
		OnRunTestsInit:         cfg.OnRunLocalTestsInit,
	})
	handlers := cfg.ExtraSSHHandlers
	handlers = append(handlers, defaultHandlers(cfg)...)
	handlers = append(handlers, runner.SSHHandlers(LocalTestRunnerPath)...)
	return fakesshserver.Start(userKey, hostKey, handlers)
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
