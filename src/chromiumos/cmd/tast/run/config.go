// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/command"
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

// testType describes the type of test to run.
type testType int

const (
	// localType represents tests that run directly on the DUT.
	localType testType = iota
	// remoteType represents tests that run on the same machine as the tast command and interact
	// with the DUT via a network connection.
	remoteType
)

// testDepsMode describes when individual tests' dependencies should be checked against software
// features supported by the DUT.
type testDepsMode int

const (
	// checkTestDepsAuto indicates that tests' dependencies should be checked when a boolean
	// expression is used to select tests but not when individual tests have been specified.
	// Dependencies are also not checked if the DUT doesn't know its supported features
	// (e.g. if it's using a non-test system image).
	checkTestDepsAuto testDepsMode = iota
	// checkTestDepsAlways indicates that tests' dependencies should always be checked.
	checkTestDepsAlways
	// checkTestDepsNever indicates that tests' dependencies should never be checked.
	checkTestDepsNever
)

const (
	defaultKeyFile     = "chromite/ssh_keys/testing_rsa"      // default private SSH key within Chrome OS checkout
	defaultOverlayPath = "src/third_party/chromiumos-overlay" // default overlay directory containing bundle ebuild
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

	build                 bool         // rebuild (and push, for local tests) a single test bundle
	buildType             testType     // type of tests to build and deploy; only used if build is true
	buildCfg              build.Config // configuration for building test bundles; only used if build is true
	buildBundle           string       // name of the test bundle to rebuild (e.g. "cros"); only used if build is true
	checkPortageDeps      bool         // check whether test bundle's dependencies are installed before building
	forceBuildLocalRunner bool         // force local_test_runner to be built and deployed even if it already exists on DUT
	overlayDir            string       // base overlay directory (e.g. chromiumos-overlay) containing bundle's ebuild
	externalDataDir       string       // dir used to cache external data files

	remoteRunner    string // path to executable that runs remote test bundles
	remoteBundleDir string // dir where packaged remote test bundles are installed
	remoteDataDir   string // dir containing packaged remote test data

	hst *host.SSH // cached SSH connection; may be nil

	checkTestDeps               testDepsMode // when test dependencies should be checked
	availableSoftwareFeatures   []string     // features supported by the DUT
	unavailableSoftwareFeatures []string     // features unsupported by the DUT

	collectSysInfo bool                 // collect system info (logs, crashes, etc.) generated during testing
	initialSysInfo *runner.SysInfoState // initial state of system info (logs, crashes, etc.) on DUT before testing

	msgTimeout             time.Duration // timeout for reading control messages; default used if zero
	localRunnerWaitTimeout time.Duration // timeout for waiting for local_test_runner to exit; default used if zero

	// Base path prepended to paths on hst when performing file copies. Only relevant for unit
	// tests, which can set this to a temp dir in order to inspect files that are copied to hst and
	// control the files that are copied from it.
	hstCopyBasePath string
	// Assigned to hst.AnnounceCmd while file copies are being performed. Only relevant for unit
	// tests, which can set this to SSHServer.NextCmd from tast/host/test so that the commands
	// that perform copies will actually be executed.
	hstCopyAnnounceCmd func(string)
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
	f.StringVar(&c.buildBundle, "buildbundle", "cros", "name of test bundle to build")
	f.BoolVar(&c.checkPortageDeps, "checkbuilddeps", true, "check test bundle's dependencies before building")
	f.BoolVar(&c.forceBuildLocalRunner, "buildlocalrunner", false, "force building local_test_runner and pushing to DUT")

	bt := command.NewEnumFlag(map[string]int{"local": int(localType), "remote": int(remoteType)},
		func(v int) { c.buildType = testType(v) }, "local")
	f.Var(bt, "buildtype", fmt.Sprintf("type of tests to build (%s; default %q)", bt.QuotedValues(), bt.Default()))

	// These are configurable since files may be installed elsewhere when running in the lab.
	f.StringVar(&c.remoteRunner, "remoterunner", "/usr/bin/remote_test_runner", "executable that runs remote test bundles")
	f.StringVar(&c.remoteBundleDir, "remotebundledir", "/usr/libexec/tast/bundles/remote", "directory containing builtin remote test bundles")
	f.StringVar(&c.remoteDataDir, "remotedatadir", "/usr/share/tast/data", "directory containing builtin remote test data")

	// Some flags are only relevant if we're running tests rather than listing them.
	if c.Mode == RunTestsMode {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
		f.BoolVar(&c.collectSysInfo, "sysinfo", true, "collect system information (logs, crashes, etc.)")
		f.StringVar(&c.overlayDir, "overlaydir", filepath.Join(trunkDir, defaultOverlayPath),
			"base overlay directory containing test bundle ebuild")
		f.StringVar(&c.externalDataDir, "externaldatadir", "/tmp/tast/external_data",
			"directory used to cache external data files")

		vals := map[string]int{
			"auto":   int(checkTestDepsAuto),
			"always": int(checkTestDepsAlways),
			"never":  int(checkTestDepsNever),
		}
		td := command.NewEnumFlag(vals, func(v int) { c.checkTestDeps = testDepsMode(v) }, "auto")
		desc := fmt.Sprintf("skip tests with software dependencies unsatisfied by DUT (%s; default %q)",
			td.QuotedValues(), td.Default())
		f.Var(td, "checktestdeps", desc)
	} else {
		c.checkTestDeps = checkTestDepsNever
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
