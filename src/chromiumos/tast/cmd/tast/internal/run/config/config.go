// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package config defines common structs to carry configuration and associated
// stateful data.
package config

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/build"
	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run/devserver"
	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/ssh"
)

// Mode describes the action to perform.
type Mode int

const (
	// RunTestsMode indicates that tests should be run and their results reported.
	RunTestsMode Mode = iota
	// ListTestsMode indicates that tests should only be listed.
	ListTestsMode
)

// ProxyMode describes how proxies should be used when running tests.
type ProxyMode int

const (
	// ProxyEnv indicates that the HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables
	// (and their lowercase counterparts) should be forwarded to the DUT if set on the host.
	ProxyEnv ProxyMode = iota
	// ProxyNone indicates that proxies shouldn't be used by the DUT.
	ProxyNone
)

const (
	defaultKeyFile     = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
	checkDepsCacheFile = "check_deps_cache.v2.json"      // file in BuildOutDir where dependency-checking results are cached
)

// Config contains shared configuration information for running or listing tests.
type Config struct {
	// Logger is used to log progress.
	Logger *logging.Logger
	// KeyFile is the path to a private SSH key to use to connect to the target device.
	KeyFile string
	// KeyDir is a directory containing private SSH keys (typically $HOME/.ssh).
	KeyDir string
	// Target is the target device for testing, in the form "[<user>@]host[:<port>]".
	Target string
	// Patterns specifies the patterns of tests to operate against.
	Patterns []string
	// ResDir is the path to the directory where test results should be written.
	// It is only used for RunTestsMode.
	ResDir string
	// TestNamesToSkip are tests that match patterns but are not sent to runners to run.
	TestNamesToSkip []string

	TestsToRun []*jsonprotocol.EntityResult // tests to be run

	Mode     Mode   // action to perform
	TastDir  string // base directory under which files are written
	TrunkDir string // path to Chrome OS checkout

	RunLocal  bool // whether to run local tests
	RunRemote bool // whether to run remote tests

	Build bool // rebuild (and push, for local tests) a single test bundle

	// Variables in this section take effect when build=true.
	BuildBundle        string // name of the test bundle to rebuild (e.g. "cros")
	BuildWorkspace     string // path to workspace containing test bundle source code
	BuildOutDir        string // path to base directory under which executables are stored
	CheckPortageDeps   bool   // check whether test bundle's dependencies are installed before building
	InstallPortageDeps bool   // install old or missing test bundle dependencies; no-op if CheckPortageDeps is false

	UseEphemeralDevserver  bool                 // start an ephemeral devserver if no devserver is specified
	ExtraAllowedBuckets    []string             // extra Google Cloud Storage buckets ephemeral devserver is allowed to access
	Devservers             []string             // list of devserver URLs; set by -devservers but may be dynamically modified
	BuildArtifactsURL      string               // Google Cloud Storage URL of build artifacts
	DownloadPrivateBundles bool                 // whether to download private bundles if missing
	DownloadMode           planner.DownloadMode // strategy to download external data files
	TLWServer              string               // address of the TLW server if available
	ReportsServer          string               // address of Reports server if available

	LocalRunner    string // path to executable that runs local test bundles
	LocalBundleDir string // dir where packaged local test bundles are installed
	LocalDataDir   string // dir containing packaged local test data
	LocalOutDir    string // dir where intermediate outputs of local tests are written

	RemoteRunner        string // path to executable that runs remote test bundles
	RemoteBundleDir     string // dir where packaged remote test bundles are installed
	RemoteFixtureServer string // path to executable that runs remote fixture server
	RemoteDataDir       string // dir containing packaged remote test data
	RemoteOutDir        string // dir where intermediate outputs of remote tests are written

	TotalShards int // total number of shards to be used in a test run
	ShardIndex  int // specifies the index of shard to used in the current run

	SSHRetries           int       // number of SSH connect retries
	ContinueAfterFailure bool      // try to run remaining local tests after bundle/DUT crash or lost SSH connection
	CheckTestDeps        bool      // whether test dependencies should be checked
	WaitUntilReady       bool      // whether to wait for DUT to be ready before running tests
	ExtraUSEFlags        []string  // additional USE flags to inject when determining features
	Proxy                ProxyMode // how proxies should be used
	CollectSysInfo       bool      // collect system info (logs, crashes, etc.) generated during testing
	MaxTestFailures      int       // maximum number of test failures

	TestVars        map[string]string // names and values of variables used to pass out-of-band data to tests
	VarsFiles       []string          // paths to variable files
	DefaultVarsDirs []string          // dirs containing default variable files

	MsgTimeout             time.Duration // timeout for reading control messages; default used if zero
	LocalRunnerWaitTimeout time.Duration // timeout for waiting for local_test_runner to exit; default used if zero

	// Base path prepended to paths on hst when performing file copies. Only relevant for unit
	// tests, which can set this to a temp dir in order to inspect files that are copied to hst and
	// control the files that are copied from it.
	HstCopyBasePath string
}

// State hold state attributes which are accumulated over the course of the run.
type State struct {
	TargetArch               string                     // architecture of target userland (usually given by "uname -m", but may be different)
	StartedRun               bool                       // true if we got to the point where we started trying to execute tests
	InitBootID               string                     // boot_id at the initial SSH connection
	Hst                      *ssh.Conn                  // cached SSH connection to DUT; may be nil
	EphemeralDevserver       *devserver.Ephemeral       // cached devserver; may be nil
	InitialSysInfo           *runner.SysInfoState       // initial state of system info (logs, crashes, etc.) on DUT before testing
	SoftwareFeatures         *dep.SoftwareFeatures      // software features of the DUT
	DeviceConfig             *device.Config             // hardware features of the DUT. Deprecated. Use HardwareFeatures instead.
	HardwareFeatures         *configpb.HardwareFeatures // hardware features of the DUT.
	OSVersion                string                     // Chrome OS Version
	FailuresCount            int                        // the number of test failures so far.
	TLWConn                  *grpc.ClientConn           // TLW gRPC service connection
	TLWServerForDUT          string                     // TLW address accessible from DUT.
	LocalDevservers          []string                   // list of devserver URLs used by local tests.
	RemoteDevservers         []string                   // list of devserver URLs used by remote tests.
	DefaultBuildArtifactsURL string                     // default URL of build artifacts.

	// gRPC Reports Client related variables.
	ReportsConn      *grpc.ClientConn       // Reports gRPC service connection.
	ReportsClient    protocol.ReportsClient // Reports gRPC client.
	ReportsLogStream protocol.Reports_LogStreamClient
}

// NewConfig returns a new configuration for executing test runners in the supplied mode.
// It sets fields that are required by SetFlags.
// tastDir is the base directory under which files are written (e.g. /tmp/tast).
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func NewConfig(mode Mode, tastDir, trunkDir string) *Config {
	return &Config{
		Mode:     mode,
		TastDir:  tastDir,
		TrunkDir: trunkDir,
		TestVars: make(map[string]string),
	}
}

// SetFlags adds common run-related flags to f that store values in Config.
func (c *Config) SetFlags(f *flag.FlagSet) {
	kf := filepath.Join(c.TrunkDir, defaultKeyFile)
	if _, err := os.Stat(kf); err != nil {
		kf = ""
	}
	f.StringVar(&c.KeyFile, "keyfile", kf, "path to private SSH key")

	kd := filepath.Join(os.Getenv("HOME"), ".ssh")
	if _, err := os.Stat(kd); err != nil {
		kd = ""
	}
	f.StringVar(&c.KeyDir, "keydir", kd, "directory containing SSH keys")

	f.BoolVar(&c.Build, "build", true, "build and push test bundle")
	f.StringVar(&c.BuildBundle, "buildbundle", "cros", "name of test bundle to build")
	f.StringVar(&c.BuildWorkspace, "buildworkspace", "", "path to Go workspace containing test bundle source code, inferred if empty")
	f.StringVar(&c.BuildOutDir, "buildoutdir", filepath.Join(c.TastDir, "build"), "directory where compiled executables are saved")
	f.BoolVar(&c.CheckPortageDeps, "checkbuilddeps", true, "check test bundle's dependencies before building")
	f.BoolVar(&c.InstallPortageDeps, "installbuilddeps", true, "automatically install/upgrade test bundle dependencies (requires -checkbuilddeps)")
	f.Var(command.NewListFlag(",", func(v []string) { c.Devservers = v }, nil), "devservers", "comma-separated list of devserver URLs")
	f.BoolVar(&c.UseEphemeralDevserver, "ephemeraldevserver", true, "start an ephemeral devserver if no devserver is specified")
	f.Var(command.NewListFlag(",", func(v []string) { c.ExtraAllowedBuckets = v }, nil), "extraallowedbuckets", "comma-separated list of extra Google Cloud Storage buckets ephemeral devserver is allowed to access")
	f.StringVar(&c.BuildArtifactsURL, "buildartifactsurl", "", "override Google Cloud Storage URL of build artifacts (implies -extraallowedbuckets)")
	f.BoolVar(&c.DownloadPrivateBundles, "downloadprivatebundles", false, "download private bundles if missing")
	ddfs := map[string]int{
		"batch": int(planner.DownloadBatch),
		"lazy":  int(planner.DownloadLazy),
	}
	ddf := command.NewEnumFlag(ddfs, func(v int) { c.DownloadMode = planner.DownloadMode(v) }, "batch")
	f.Var(ddf, "downloaddata", fmt.Sprintf("strategy to download external data files (%s; default %q)", ddf.QuotedValues(), ddf.Default()))
	f.BoolVar(&c.ContinueAfterFailure, "continueafterfailure", true, "try to run remaining tests after bundle/DUT crash or lost SSH connection")
	f.IntVar(&c.SSHRetries, "sshretries", 0, "number of SSH connect retries")
	f.StringVar(&c.TLWServer, "tlwserver", "", "TLW server address")
	f.StringVar(&c.ReportsServer, "reports_server", "", "Reports server address")
	f.IntVar(&c.MaxTestFailures, "maxtestfailures", 0, "the maximum number test failures allowed (default to 0 which means no limit)")

	f.IntVar(&c.TotalShards, "totalshards", 1, "total number of shards to be used in a test run")
	f.IntVar(&c.ShardIndex, "shardindex", 0, "the index of shard to used in the current run")

	f.StringVar(&c.LocalRunner, "localrunner", "", "executable that runs local test bundles")
	f.StringVar(&c.LocalBundleDir, "localbundledir", "", "directory containing builtin local test bundles")
	f.StringVar(&c.LocalDataDir, "localdatadir", "", "directory containing builtin local test data")
	f.StringVar(&c.LocalOutDir, "localoutdir", "", "directory where intermediate test outputs are written")

	// These are configurable since files may be installed elsewhere when running in the lab.
	f.StringVar(&c.RemoteRunner, "remoterunner", "", "executable that runs remote test bundles")
	f.StringVar(&c.RemoteBundleDir, "remotebundledir", "", "directory containing builtin remote test bundles")
	f.StringVar(&c.RemoteDataDir, "remotedatadir", "", "directory containing builtin remote test data")

	// Some flags are only relevant if we're running tests rather than listing them.
	if c.Mode == RunTestsMode {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
		f.BoolVar(&c.CollectSysInfo, "sysinfo", true, "collect system information (logs, crashes, etc.)")
		f.BoolVar(&c.WaitUntilReady, "waituntilready", true, "wait until DUT is ready before running tests")
		f.BoolVar(&c.CheckTestDeps, "checktestdeps", true, "skip tests with software dependencies unsatisfied by DUT")

		f.Var(command.NewListFlag(",", func(v []string) { c.ExtraUSEFlags = v }, nil), "extrauseflags",
			"comma-separated list of additional USE flags to inject when checking test dependencies")

		vf := command.RepeatedFlag(func(v string) error {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				return errors.New(`want "name=value"`)
			}
			c.TestVars[parts[0]] = parts[1]
			return nil
		})
		f.Var(&vf, "var", `runtime variable to pass to tests, as "name=value" (can be repeated)`)
		dvd := command.RepeatedFlag(func(path string) error {
			c.DefaultVarsDirs = append(c.DefaultVarsDirs, path)
			return nil
		})
		f.Var(&dvd, "defaultvarsdir", "directory having YAML files containing variables (can be repeated)")
		vff := command.RepeatedFlag(func(path string) error {
			c.VarsFiles = append(c.VarsFiles, path)
			return nil
		})
		f.Var(&vff, "varsfile", "YAML file containing variables (can be repeated)")

		vals := map[string]int{
			"env":  int(ProxyEnv),
			"none": int(ProxyNone),
		}
		td := command.NewEnumFlag(vals, func(v int) { c.Proxy = ProxyMode(v) }, "env")
		desc := fmt.Sprintf("proxy settings used by the DUT (%s; default %q)", td.QuotedValues(), td.Default())
		f.Var(td, "proxy", desc)
	} else {
		c.CheckTestDeps = false
	}
}

// CloseEphemeralDevserver closes and resets s.EphemeralDevserver if non-nil.
func (s *State) CloseEphemeralDevserver(ctx context.Context) error {
	var err error
	if s.EphemeralDevserver != nil {
		err = s.EphemeralDevserver.Close(ctx)
		s.EphemeralDevserver = nil
	}
	return err
}

// Close releases the config's resources (e.g. cached SSH connections).
// It should be called at the completion of testing.
func (s *State) Close(ctx context.Context) error {
	s.CloseEphemeralDevserver(ctx) // ignore error; not meaningful if c.hst is dead
	var firstErr error
	if s.Hst != nil {
		if err := s.Hst.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		s.Hst = nil
	}
	if s.TLWConn != nil {
		if err := s.TLWConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.TLWConn = nil
	}
	if s.ReportsConn != nil {
		if err := s.ReportsConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.ReportsConn = nil
		s.ReportsClient = nil
	}
	return firstErr
}

// DeriveDefaults sets default config values to unset members, possibly deriving from
// already set members. It should be called after non-default values are set to c.
func (c *Config) DeriveDefaults() error {
	setIfEmpty := func(p *string, s string) {
		if *p == "" {
			*p = s
		}
	}

	setIfEmpty(&c.ResDir, filepath.Join(c.TastDir, "results", time.Now().Format("20060102-150405")))

	b := getKnownBundleInfo(c.BuildBundle)
	if b == nil {
		if c.BuildWorkspace == "" {
			return fmt.Errorf("unknown bundle %q; please set -buildworkspace explicitly", c.BuildBundle)
		}
	} else {
		setIfEmpty(&c.BuildWorkspace, filepath.Join(c.TrunkDir, b.workspace))
	}

	// Generate a timestamped directory path to always create a new one.
	ts := time.Now().Format("20060102-150405.000000000")
	setIfEmpty(&c.LocalOutDir, fmt.Sprintf("/usr/local/tmp/tast_out.%s", ts))
	// RemoteOutDir should be under ResDir so that we can move files with os.Rename (crbug.com/813282).
	setIfEmpty(&c.RemoteOutDir, fmt.Sprintf("%s/tast_out.%s", c.ResDir, ts))

	if c.Build {
		// If -build=true, use different paths than -build=false to avoid overwriting
		// Portage-managed files.
		setIfEmpty(&c.LocalRunner, "/usr/local/libexec/tast/bin_pushed/local_test_runner")
		setIfEmpty(&c.LocalBundleDir, "/usr/local/libexec/tast/bundles/local_pushed")
		setIfEmpty(&c.LocalDataDir, "/usr/local/share/tast/data_pushed")
		setIfEmpty(&c.RemoteRunner, filepath.Join(c.BuildOutDir, build.ArchHost, "remote_test_runner"))
		setIfEmpty(&c.RemoteBundleDir, filepath.Join(c.BuildOutDir, build.ArchHost, build.RemoteBundleBuildSubdir))
		// Remote data files are read from the source checkout directly.
		setIfEmpty(&c.RemoteDataDir, filepath.Join(c.BuildWorkspace, "src"))
		// Build and run local/remote tests only when the corresponding package exists.
		if _, err := os.Stat(filepath.Join(c.BuildWorkspace, "src", build.LocalBundlePkgPathPrefix, c.BuildBundle)); err == nil {
			c.RunLocal = true
		}
		if _, err := os.Stat(filepath.Join(c.BuildWorkspace, "src", build.RemoteBundlePkgPathPrefix, c.BuildBundle)); err == nil {
			c.RunRemote = true
		}
		if !c.RunLocal && !c.RunRemote {
			return fmt.Errorf("could not find bundle %q at %s", c.BuildBundle, c.BuildWorkspace)
		}
	} else {
		// If -build=false, default values are paths to files installed by Portage.
		setIfEmpty(&c.LocalRunner, "/usr/local/bin/local_test_runner")
		setIfEmpty(&c.LocalBundleDir, "/usr/local/libexec/tast/bundles/local")
		setIfEmpty(&c.LocalDataDir, "/usr/local/share/tast/data")
		setIfEmpty(&c.RemoteRunner, "/usr/bin/remote_test_runner")
		setIfEmpty(&c.RemoteBundleDir, "/usr/libexec/tast/bundles/remote")
		setIfEmpty(&c.RemoteDataDir, "/usr/share/tast/data")
		// Always run both local and remote tests.
		c.RunLocal = true
		c.RunRemote = true
	}
	// TODO(crbug/1177189): we assume there's only one remote bundle. Consider
	// removing the restriction.
	c.RemoteFixtureServer = filepath.Join(c.RemoteBundleDir, "cros")

	// Apply -varsfile.
	for _, path := range c.VarsFiles {
		if err := readAndMergeVarsFile(c.TestVars, path, errorOnDuplicate); err != nil {
			return fmt.Errorf("failed to apply vars from %s: %v", path, err)
		}
	}

	// Apply variables from default configurations.
	if len(c.DefaultVarsDirs) == 0 {
		if c.Build {
			c.DefaultVarsDirs = []string{
				filepath.Join(c.TrunkDir, "src/platform/tast-tests-private/vars"),
				filepath.Join(c.TrunkDir, "src/platform/tast-tests/vars"),
			}
		} else {
			c.DefaultVarsDirs = []string{
				"/etc/tast/vars/private",
				"/etc/tast/vars/public",
			}
		}
	}
	var defaultVarsFiles []string
	for _, d := range c.DefaultVarsDirs {
		fs, err := findVarsFiles(d)
		if err != nil {
			return fmt.Errorf("failed to find vars files under %s: %v", d, err)
		}
		defaultVarsFiles = append(defaultVarsFiles, fs...)
	}
	defaultVars := make(map[string]string)
	for _, path := range defaultVarsFiles {
		if err := readAndMergeVarsFile(defaultVars, path, errorOnDuplicate); err != nil {
			return fmt.Errorf("failed to apply vars from %s: %v", path, err)
		}
	}
	mergeVars(c.TestVars, defaultVars, skipOnDuplicate) // -var and -varsfile override defaults

	if c.BuildArtifactsURL != "" {
		if !strings.HasSuffix(c.BuildArtifactsURL, "/") {
			return errors.New("-buildartifactsurl must end with a slash")
		}

		// Add the bucket to the extra allowed bucket list.
		u, err := url.Parse(c.BuildArtifactsURL)
		if err != nil {
			return fmt.Errorf("failed to parse -buildartifactsurl: %v", err)
		}
		if u.Scheme != "gs" {
			return errors.New("invalid -buildartifactsurl: not a gs:// URL")
		}
		c.ExtraAllowedBuckets = append(c.ExtraAllowedBuckets, u.Host)
	}
	if c.TotalShards < 1 {
		return fmt.Errorf("%v is an invalid number of shards", c.ShardIndex)
	}
	if c.ShardIndex < 0 || c.ShardIndex >= c.TotalShards {
		return fmt.Errorf("shard index %v is out of range", c.ShardIndex)
	}

	return nil
}

// BuildCfg returns a build.Config.
func (c *Config) BuildCfg() *build.Config {
	return &build.Config{
		Logger:             c.Logger,
		CheckBuildDeps:     c.CheckPortageDeps,
		CheckDepsCachePath: filepath.Join(c.BuildOutDir, checkDepsCacheFile),
		InstallPortageDeps: c.InstallPortageDeps,
		TastWorkspace:      c.tastWorkspace(),
	}
}

// CommonWorkspaces returns Go workspaces containing source code needed to build all Tast-related executables.
func (c *Config) CommonWorkspaces() []string {
	return []string{
		c.tastWorkspace(), // shared code
		"/usr/lib/gopath", // system packages
	}
}

// tastWorkspace returns the Go workspace containing the Tast framework.
func (c *Config) tastWorkspace() string {
	return filepath.Join(c.TrunkDir, "src/platform/tast")
}

// crosTestWorkspace returns the Go workspace containing standard test-related code.
// This workspace also contains the default "cros" test bundles.
func (c *Config) crosTestWorkspace() string {
	return filepath.Join(c.TrunkDir, "src/platform/tast-tests")
}

// BundleWorkspaces returns Go workspaces containing source code needed to build c.BuildBundle.
func (c *Config) BundleWorkspaces() []string {
	ws := []string{c.crosTestWorkspace()}
	ws = append(ws, c.CommonWorkspaces()...)

	// If a custom test bundle workspace was specified, prepend it.
	if c.BuildWorkspace != ws[0] {
		ws = append([]string{c.BuildWorkspace}, ws...)
	}
	return ws
}

// LocalBundleGlob returns a file path glob that matches local test bundle executables.
func (c *Config) LocalBundleGlob() string {
	return c.bundleGlob(c.LocalBundleDir)
}

// RemoteBundleGlob returns a file path glob that matches remote test bundle executables.
func (c *Config) RemoteBundleGlob() string {
	return c.bundleGlob(c.RemoteBundleDir)
}

func (c *Config) bundleGlob(dir string) string {
	last := "*"
	if c.Build {
		last = c.BuildBundle
	}
	return filepath.Join(dir, last)
}
