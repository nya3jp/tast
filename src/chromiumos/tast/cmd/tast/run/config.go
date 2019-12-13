// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/cmd/tast/build"
	"chromiumos/tast/cmd/tast/logging"
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

// proxyMode describes how proxies should be used when running tests.
type proxyMode int

const (
	// proxyEnv indicates that the HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables
	// (and their lowercase counterparts) should be forwarded to the DUT if set on the host.
	proxyEnv proxyMode = iota
	// proxyNone indicates that proxies shouldn't be used by the DUT.
	proxyNone
)

const (
	defaultKeyFile     = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
	checkDepsCacheFile = "check_deps_cache.v2.json"      // file in buildOutDir where dependency-checking results are cached
)

// Config contains shared configuration information for running or listing tests.
type Config struct {
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

	mode     Mode   // action to perform
	tastDir  string // base directory under which files are written
	trunkDir string // path to Chrome OS checkout

	runLocal  bool // whether to run local tests
	runRemote bool // whether to run remote tests

	build bool // rebuild (and push, for local tests) a single test bundle

	// Variables in this section take effect when build=true.
	buildBundle        string // name of the test bundle to rebuild (e.g. "cros")
	buildWorkspace     string // path to workspace containing test bundle source code
	buildOutDir        string // path to base directory under which executables are stored
	checkPortageDeps   bool   // check whether test bundle's dependencies are installed before building
	installPortageDeps bool   // install old or missing test bundle dependencies; no-op if checkPortageDeps is false

	useEphemeralDevserver  bool     // start an ephemeral devserver if no devserver is specified
	devservers             []string // list of devserver URLs; set by -devservers but may be dynamically modified
	downloadPrivateBundles bool     // whether to download private bundles if missing

	localRunner    string // path to executable that runs local test bundles
	localBundleDir string // dir where packaged local test bundles are installed
	localDataDir   string // dir containing packaged local test data
	localOutDir    string // dir where intermediate outputs of local tests are written

	remoteRunner    string // path to executable that runs remote test bundles
	remoteBundleDir string // dir where packaged remote test bundles are installed
	remoteDataDir   string // dir containing packaged remote test data
	remoteOutDir    string // dir where intermediate outputs of remote tests are written

	sshRetries           int       // number of SSH connect retries
	continueAfterFailure bool      // try to run remaining local tests after bundle/DUT crash or lost SSH connection
	checkTestDeps        bool      // whether test dependencies should be checked
	waitUntilReady       bool      // whether to wait for DUT to be ready before running tests
	extraUSEFlags        []string  // additional USE flags to inject when determining features
	proxy                proxyMode // how proxies should be used
	collectSysInfo       bool      // collect system info (logs, crashes, etc.) generated during testing

	testVars       map[string]string // names and values of variables used to pass out-of-band data to tests
	varsFiles      []string          // paths to variable files
	defaultVarsDir string            // dir containing default variable files

	msgTimeout             time.Duration // timeout for reading control messages; default used if zero
	localRunnerWaitTimeout time.Duration // timeout for waiting for local_test_runner to exit; default used if zero

	// Base path prepended to paths on hst when performing file copies. Only relevant for unit
	// tests, which can set this to a temp dir in order to inspect files that are copied to hst and
	// control the files that are copied from it.
	hstCopyBasePath string
	// Assigned to hst.AnnounceCmd while file copies are being performed. Only relevant for unit
	// tests, which can assign this to SSHServer.NextRealCmd from tast/host/test so that the commands
	// that perform copies will actually be executed.
	hstCopyAnnounceCmd func(string)

	// The following fields hold state that is accumulated over the course of the run.
	// TODO(crbug.com/971517): Consider moving these fields into a separate struct,
	// as they aren't really configuration.
	targetArch                  string               // architecture of target userland (usually given by "uname -m", but may be different)
	startedRun                  bool                 // true if we got to the point where we started trying to execute tests
	initBootID                  string               // boot_id at the initial SSH connection
	hst                         *host.SSH            // cached SSH connection to DUT; may be nil
	ephemeralDevserver          *ephemeralDevserver  // cached devserver; may be nil
	initialSysInfo              *runner.SysInfoState // initial state of system info (logs, crashes, etc.) on DUT before testing
	availableSoftwareFeatures   []string             // features supported by the DUT
	unavailableSoftwareFeatures []string             // features unsupported by the DUT
}

// NewConfig returns a new configuration for executing test runners in the supplied mode.
// It sets fields that are required by SetFlags.
// tastDir is the base directory under which files are written (e.g. /tmp/tast).
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func NewConfig(mode Mode, tastDir, trunkDir string) *Config {
	return &Config{
		mode:     mode,
		tastDir:  tastDir,
		trunkDir: trunkDir,
		testVars: make(map[string]string),
	}
}

// SetFlags adds common run-related flags to f that store values in Config.
func (c *Config) SetFlags(f *flag.FlagSet) {
	kf := filepath.Join(c.trunkDir, defaultKeyFile)
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
	f.StringVar(&c.buildWorkspace, "buildworkspace", "", "path to Go workspace containing test bundle source code, inferred if empty")
	f.StringVar(&c.buildOutDir, "buildoutdir", filepath.Join(c.tastDir, "build"), "directory where compiled executables are saved")
	f.BoolVar(&c.checkPortageDeps, "checkbuilddeps", true, "check test bundle's dependencies before building")
	f.BoolVar(&c.installPortageDeps, "installbuilddeps", true, "automatically install/upgrade test bundle dependencies (requires -checkbuilddeps)")
	f.Var(command.NewListFlag(",", func(v []string) { c.devservers = v }, nil), "devservers", "comma-separated list of devserver URLs")
	f.BoolVar(&c.useEphemeralDevserver, "ephemeraldevserver", true, "start an ephemeral devserver if no devserver is specified")
	f.BoolVar(&c.downloadPrivateBundles, "downloadprivatebundles", false, "download private bundles if missing")
	f.BoolVar(&c.continueAfterFailure, "continueafterfailure", false, "try to run remaining tests after bundle/DUT crash or lost SSH connection")
	f.IntVar(&c.sshRetries, "sshretries", 0, "number of SSH connect retries")

	f.StringVar(&c.localRunner, "localrunner", "", "executable that runs local test bundles")
	f.StringVar(&c.localBundleDir, "localbundledir", "", "directory containing builtin local test bundles")
	f.StringVar(&c.localDataDir, "localdatadir", "", "directory containing builtin local test data")
	f.StringVar(&c.localOutDir, "localoutdir", "", "directory where intermediate test outputs are written")

	// These are configurable since files may be installed elsewhere when running in the lab.
	f.StringVar(&c.remoteRunner, "remoterunner", "", "executable that runs remote test bundles")
	f.StringVar(&c.remoteBundleDir, "remotebundledir", "", "directory containing builtin remote test bundles")
	f.StringVar(&c.remoteDataDir, "remotedatadir", "", "directory containing builtin remote test data")

	// Some flags are only relevant if we're running tests rather than listing them.
	if c.mode == RunTestsMode {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
		f.BoolVar(&c.collectSysInfo, "sysinfo", true, "collect system information (logs, crashes, etc.)")
		f.BoolVar(&c.waitUntilReady, "waituntilready", false, "wait until DUT is ready before running tests")
		f.BoolVar(&c.checkTestDeps, "checktestdeps", true, "skip tests with software dependencies unsatisfied by DUT")

		f.Var(command.NewListFlag(",", func(v []string) { c.extraUSEFlags = v }, nil), "extrauseflags",
			"comma-separated list of additional USE flags to inject when checking test dependencies")

		vf := command.RepeatedFlag(func(v string) error {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				return errors.New(`want "name=value"`)
			}
			c.testVars[parts[0]] = parts[1]
			return nil
		})
		f.Var(&vf, "var", `runtime variable to pass to tests, as "name=value" (can be repeated)`)
		f.StringVar(&c.defaultVarsDir, "defaultvarsdir", "", "directory having YAML files containing variables")
		vff := command.RepeatedFlag(func(path string) error {
			c.varsFiles = append(c.varsFiles, path)
			return nil
		})
		f.Var(&vff, "varsfile", "YAML file containing variables (can be repeated)")

		vals := map[string]int{
			"env":  int(proxyEnv),
			"none": int(proxyNone),
		}
		td := command.NewEnumFlag(vals, func(v int) { c.proxy = proxyMode(v) }, "env")
		desc := fmt.Sprintf("proxy settings used by the DUT (%s; default %q)", td.QuotedValues(), td.Default())
		f.Var(td, "proxy", desc)
	} else {
		c.checkTestDeps = false
	}
}

// Close releases the config's resources (e.g. cached SSH connections).
// It should be called at the completion of testing.
func (c *Config) Close(ctx context.Context) error {
	closeEphemeralDevserver(ctx, c) // ignore error; not meaningful if c.hst is dead
	var err error
	if c.hst != nil {
		err = c.hst.Close(ctx)
		c.hst = nil
	}
	return err
}

// DeriveDefaults sets default config values to unset members, possibly deriving from
// already set members. It should be called after non-default values are set to c.
func (c *Config) DeriveDefaults() error {
	setIfEmpty := func(p *string, s string) {
		if *p == "" {
			*p = s
		}
	}

	setIfEmpty(&c.ResDir, filepath.Join(c.tastDir, "results", time.Now().Format("20060102-150405")))

	b := getKnownBundleInfo(c.buildBundle)
	if b == nil {
		if c.buildWorkspace == "" {
			return fmt.Errorf("unknown bundle %q; please set -buildworkspace explicitly", c.buildBundle)
		}
	} else {
		setIfEmpty(&c.buildWorkspace, filepath.Join(c.trunkDir, b.workspace))
	}

	// Generate a timestamped directory path to always create a new one.
	ts := time.Now().Format("20060102-150405.000000000")
	setIfEmpty(&c.localOutDir, fmt.Sprintf("/usr/local/tmp/tast_out.%s", ts))
	// remoteOutDir should be under ResDir so that we can move files with os.Rename (crbug.com/813282).
	setIfEmpty(&c.remoteOutDir, fmt.Sprintf("%s/tast_out.%s", c.ResDir, ts))

	if c.build {
		// If -build=true, use different paths than -build=false to avoid overwriting
		// Portage-managed files.
		setIfEmpty(&c.localRunner, "/usr/local/libexec/tast/bin_pushed/local_test_runner")
		setIfEmpty(&c.localBundleDir, "/usr/local/libexec/tast/bundles/local_pushed")
		setIfEmpty(&c.localDataDir, "/usr/local/share/tast/data_pushed")
		setIfEmpty(&c.remoteRunner, filepath.Join(c.buildOutDir, build.ArchHost, "remote_test_runner"))
		setIfEmpty(&c.remoteBundleDir, filepath.Join(c.buildOutDir, build.ArchHost, remoteBundleBuildSubdir))
		// Remote data files are read from the source checkout directly.
		setIfEmpty(&c.remoteDataDir, filepath.Join(c.buildWorkspace, "src"))
		// Build and run local/remote tests only when the corresponding package exists.
		if _, err := os.Stat(filepath.Join(c.buildWorkspace, "src", localBundlePkgPathPrefix, c.buildBundle)); err == nil {
			c.runLocal = true
		}
		if _, err := os.Stat(filepath.Join(c.buildWorkspace, "src", remoteBundlePkgPathPrefix, c.buildBundle)); err == nil {
			c.runRemote = true
		}
		if !c.runLocal && !c.runRemote {
			return fmt.Errorf("could not find bundle %q at %s", c.buildBundle, c.buildWorkspace)
		}
	} else {
		// If -build=false, default values are paths to files installed by Portage.
		setIfEmpty(&c.localRunner, "/usr/local/bin/local_test_runner")
		setIfEmpty(&c.localBundleDir, "/usr/local/libexec/tast/bundles/local")
		setIfEmpty(&c.localDataDir, "/usr/local/share/tast/data")
		setIfEmpty(&c.remoteRunner, "/usr/bin/remote_test_runner")
		setIfEmpty(&c.remoteBundleDir, "/usr/libexec/tast/bundles/remote")
		setIfEmpty(&c.remoteDataDir, "/usr/share/tast/data")
		// Always run both local and remote tests.
		c.runLocal = true
		c.runRemote = true
	}

	// Apply -varsfile.
	for _, path := range c.varsFiles {
		if err := readAndMergeVarsFile(c.testVars, path, errorOnDuplicate); err != nil {
			return fmt.Errorf("failed to apply vars from %s: %v", path, err)
		}
	}

	// Apply variables from default configurations.
	if c.build {
		setIfEmpty(&c.defaultVarsDir, filepath.Join(c.trunkDir, "src/platform/tast-tests-private/vars"))
	} else {
		setIfEmpty(&c.defaultVarsDir, "/etc/tast/vars/private")
	}
	defaultVarsFiles, err := findVarsFiles(c.defaultVarsDir)
	if err != nil {
		return fmt.Errorf("failed to find vars files under %s: %v", c.defaultVarsDir, err)
	}
	defaultVars := make(map[string]string)
	for _, path := range defaultVarsFiles {
		if err := readAndMergeVarsFile(defaultVars, path, errorOnDuplicate); err != nil {
			return fmt.Errorf("failed to apply vars from %s: %v", path, err)
		}
	}
	mergeVars(c.testVars, defaultVars, skipOnDuplicate) // -var and -varsfile override defaults

	return nil
}

// buildCfg returns a build.Config.
func (c *Config) buildCfg() *build.Config {
	return &build.Config{
		Logger:             c.Logger,
		CheckBuildDeps:     c.checkPortageDeps,
		CheckDepsCachePath: filepath.Join(c.buildOutDir, checkDepsCacheFile),
		InstallPortageDeps: c.installPortageDeps,
		TastWorkspace:      c.tastWorkspace(),
	}
}

// commonWorkspaces returns Go workspaces containing source code needed to build all Tast-related executables.
func (c *Config) commonWorkspaces() []string {
	return []string{
		c.tastWorkspace(), // shared code
		"/usr/lib/gopath", // system packages
	}
}

// tastWorkspace returns the Go workspace containing the Tast framework.
func (c *Config) tastWorkspace() string {
	return filepath.Join(c.trunkDir, "src/platform/tast")
}

// crosTestWorkspace returns the Go workspace containing standard test-related code.
// This workspace also contains the default "cros" test bundles.
func (c *Config) crosTestWorkspace() string {
	return filepath.Join(c.trunkDir, "src/platform/tast-tests")
}

// bundleWorkspaces returns Go workspaces containing source code needed to build c.buildBundle.
func (c *Config) bundleWorkspaces() []string {
	ws := []string{c.crosTestWorkspace()}
	ws = append(ws, c.commonWorkspaces()...)

	// If a custom test bundle workspace was specified, prepend it.
	if c.buildWorkspace != ws[0] {
		ws = append([]string{c.buildWorkspace}, ws...)
	}
	return ws
}
