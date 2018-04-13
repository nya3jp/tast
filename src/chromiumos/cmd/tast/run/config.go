// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/host"
	"chromiumos/tast/runner"
)

// Mode describes the action to perform.
type Mode int

const (
	// RunTestsMode indicates that tests should be run and their results reported.
	RunTestsMode Mode = iota
	// ListTestsMode indicates that tests should only be listed.
	ListTestsMode
)

const (
	localType  = "local"  // -buildtype flag value for local tests
	remoteType = "remote" // -buildtype flag value for remote tests

	defaultKeyFile = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
)

// Config contains shared configuration information for running or listing tests.
type Config struct {
	// Mode describes the action to perform.
	Mode Mode
	// Logger is used to log progress.
	Logger logging.Logger
	// KeyFile is the path to a private SSH key to use to connect to the target device.
	KeyFile string
	// KeyDir is a directory containing private SSH keys (typically $HOME/.ssh).
	KeyDir string
	// Target is the target device for testing, in the form "[<user>@]host[:<port>]".
	Target string
	// Patterns specifies which tests to operate against.
	Patterns []string
	// ResDir is the path to the directory where test results should be written.
	// It is only used for RunTestsMode.
	ResDir string

	build            bool         // rebuild (and push, for local tests) a single test bundle
	buildType        string       // type of tests to build and deploy (either "local" or "remote"); only sued if Build is true
	buildCfg         build.Config // configuration for building test bundles; only used if Build is true
	buildBundle      string       // name of the test bundle to rebuild (e.g. "cros"); only used if Build is true
	checkPortageDeps bool         // check whether test bundle's dependencies are installed before building

	remoteRunner    string // path to executable that runs remote test bundles
	remoteBundleDir string // dir where packaged remote test bundles are installed
	remoteDataDir   string // dir containing packaged remote test data

	hst *host.SSH // cached SSH connection; may be nil

	collectSysInfo bool                 // collect system info (logs, crashes, etc.) generated during testing
	initialSysInfo *runner.SysInfoState // initial state of system info (logs, crashes, etc.) on DUT before testing

	msgTimeout time.Duration // timeout for reading control messages; default used if zero
}

// SetFlags adds common run-related flags to f that store values in Config.
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func (c *Config) SetFlags(f *flag.FlagSet, trunkDir string) {
	kf := filepath.Join(trunkDir, defaultKeyFile)
	if _, err := os.Stat(kf); err != nil {
		kf = ""
	}
	f.StringVar(&c.KeyFile, "keyfile", kf, "path to private SSH key")

	kd := filepath.Join(os.Getenv("HOME"), ".ssh")
	if _, err := os.Stat(kd); err != nil {
		kd = ""
	}
	f.StringVar(&c.KeyDir, "keydir", kd, "directory containing SSH keys")

	f.BoolVar(&c.build, "build", true, "build and push test bundle")
	f.StringVar(&c.buildType, "buildtype", localType, "type of tests to build (\""+localType+"\" or \""+remoteType+"\")")
	f.StringVar(&c.buildBundle, "buildbundle", "cros", "name of test bundle to build")
	f.BoolVar(&c.checkPortageDeps, "checkbuilddeps", true, "check test bundle's dependencies before building")

	// These are configurable since files may be installed elsewhere when running in the lab.
	f.StringVar(&c.remoteRunner, "remoterunner", "/usr/bin/remote_test_runner", "executable that runs remote test bundles")
	f.StringVar(&c.remoteBundleDir, "remotebundledir", "/usr/libexec/tast/bundles/remote", "directory containing builtin remote test bundles")
	f.StringVar(&c.remoteDataDir, "remotedatadir", "/usr/share/tast/data/remote", "directory containing builtin remote test data")

	// We only need a results dir or system info if we're running tests rather than listing them.
	if c.Mode == RunTestsMode {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
		f.BoolVar(&c.collectSysInfo, "sysinfo", true, "collect system information (logs, crashes, etc.)")
	}

	c.buildCfg.SetFlags(f, trunkDir)
}

// Close releases the config's resources (e.g. cached SSH connections).
// It should be called at the completion of testing.
func (c *Config) Close(ctx context.Context) error {
	var err error
	if c.hst != nil {
		err = c.hst.Close(ctx)
		c.hst = nil
	}
	return err
}
