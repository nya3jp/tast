// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package config defines common structs to carry configuration and associated
// stateful data.
package config

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/cmd/tast/internal/build"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/internal/protocol"
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
	defaultKeyFile               = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
	checkDepsCacheFile           = "check_deps_cache.v2.json"      // file in BuildOutDir where dependency-checking results are cached
	defaultSystemServicesTimeout = 120 * time.Second               //default timeout for waiting for system services to be ready in seconds
)

// MutableConfig is similar to Config, but its fields are mutable.
// Call Freeze to obtain a Config from MutableConfig.
type MutableConfig struct {
	// See Config for descriptions of these fields.

	KeyFile  string
	KeyDir   string
	Target   string
	Patterns []string
	ResDir   string

	Mode     Mode
	TastDir  string
	TrunkDir string

	Build bool

	BuildBundle        string
	BuildWorkspace     string
	BuildOutDir        string
	CheckPortageDeps   bool
	InstallPortageDeps bool

	UseEphemeralDevserver     bool
	ExtraAllowedBuckets       []string
	Devservers                []string
	BuildArtifactsURLOverride string
	DownloadPrivateBundles    bool
	DownloadMode              protocol.DownloadMode
	TLWServer                 string
	ReportsServer             string
	CompanionDUTs             map[string]string

	LocalRunner    string
	LocalBundleDir string
	LocalDataDir   string
	LocalOutDir    string
	LocalTempDir   string

	RemoteRunner    string
	RemoteBundleDir string
	PrimaryBundle   string
	RemoteDataDir   string
	RemoteOutDir    string
	RemoteTempDir   string

	TotalShards int
	ShardIndex  int

	SSHRetries           int
	ContinueAfterFailure bool
	CheckTestDeps        bool
	WaitUntilReady       bool
	ExtraUSEFlags        []string
	Proxy                ProxyMode
	CollectSysInfo       bool
	MaxTestFailures      int
	ExcludeSkipped       bool

	TestVars         map[string]string
	VarsFiles        []string
	DefaultVarsDirs  []string
	MaybeMissingVars string

	MsgTimeout    time.Duration
	DebuggerPorts map[debugger.DebugTarget]int

	Retries int

	SystemServicesTimeout time.Duration
}

// Config contains shared configuration information for running or listing tests.
// All Config values are frozen and cannot be altered after construction.
type Config struct {
	m *MutableConfig
}

// KeyFile is the path to a private SSH key to use to connect to the target device.
func (c *Config) KeyFile() string { return c.m.KeyFile }

// KeyDir is a directory containing private SSH keys (typically $HOME/.ssh).
func (c *Config) KeyDir() string { return c.m.KeyDir }

// Target is the target device for testing, in the form "[<user>@]host[:<port>]".
func (c *Config) Target() string { return c.m.Target }

// ProtoSSHConfig returns an SSHConfig proto.
func (c *Config) ProtoSSHConfig() *protocol.SSHConfig {
	return &protocol.SSHConfig{
		ConnectionSpec: c.Target(),
		KeyFile:        c.KeyFile(),
		KeyDir:         c.KeyDir(),
	}
}

// Patterns specifies the patterns of tests to operate against.
func (c *Config) Patterns() []string { return append([]string(nil), c.m.Patterns...) }

// ResDir is the path to the directory where test results should be written. It is only used for RunTestsMode.
func (c *Config) ResDir() string { return c.m.ResDir }

// Mode is action to perform.
func (c *Config) Mode() Mode { return c.m.Mode }

// TastDir is base directory under which files are written.
func (c *Config) TastDir() string { return c.m.TastDir }

// TrunkDir is path to Chrome OS checkout.
func (c *Config) TrunkDir() string { return c.m.TrunkDir }

// Build is rebuild (and push, for local tests) a single test bundle.
func (c *Config) Build() bool { return c.m.Build }

// BuildBundle is name of the test bundle to rebuild (e.g. "cros").
func (c *Config) BuildBundle() string { return c.m.BuildBundle }

// BuildWorkspace is path to workspace containing test bundle source code.
func (c *Config) BuildWorkspace() string { return c.m.BuildWorkspace }

// BuildOutDir is path to base directory under which executables are stored.
func (c *Config) BuildOutDir() string { return c.m.BuildOutDir }

// CheckPortageDeps is check whether test bundle's dependencies are installed before building.
func (c *Config) CheckPortageDeps() bool { return c.m.CheckPortageDeps }

// InstallPortageDeps is install old or missing test bundle dependencies; no-op if CheckPortageDeps is false.
func (c *Config) InstallPortageDeps() bool { return c.m.InstallPortageDeps }

// UseEphemeralDevserver is start an ephemeral devserver if no devserver is specified.
func (c *Config) UseEphemeralDevserver() bool { return c.m.UseEphemeralDevserver }

// ExtraAllowedBuckets is extra Google Cloud Storage buckets ephemeral devserver is allowed to access.
func (c *Config) ExtraAllowedBuckets() []string {
	return append([]string(nil), c.m.ExtraAllowedBuckets...)
}

// Devservers is list of devserver URLs; set by -devservers but may be dynamically modified.
func (c *Config) Devservers() []string { return append([]string(nil), c.m.Devservers...) }

// BuildArtifactsURLOverride is Google Cloud Storage URL of build artifacts
// specified in the command line. If it is empty, it should be detected from
// the DUT.
func (c *Config) BuildArtifactsURLOverride() string { return c.m.BuildArtifactsURLOverride }

// DownloadPrivateBundles is whether to download private bundles if missing.
func (c *Config) DownloadPrivateBundles() bool { return c.m.DownloadPrivateBundles }

// DownloadMode is strategy to download external data files.
func (c *Config) DownloadMode() protocol.DownloadMode { return c.m.DownloadMode }

// TLWServer is address of the TLW server if available.
func (c *Config) TLWServer() string { return c.m.TLWServer }

// ReportsServer is address of Reports server if available.
func (c *Config) ReportsServer() string { return c.m.ReportsServer }

// CompanionDUTs is role to address mapping of companion DUTs..
func (c *Config) CompanionDUTs() map[string]string {
	duts := make(map[string]string)
	for k, v := range c.m.CompanionDUTs {
		duts[k] = v
	}
	return duts
}

// LocalRunner is path to executable that runs local test bundles.
func (c *Config) LocalRunner() string { return c.m.LocalRunner }

// LocalBundleDir is dir where packaged local test bundles are installed.
func (c *Config) LocalBundleDir() string { return c.m.LocalBundleDir }

// LocalDataDir is dir containing packaged local test data.
func (c *Config) LocalDataDir() string { return c.m.LocalDataDir }

// LocalOutDir is dir where intermediate outputs of local tests are written.
func (c *Config) LocalOutDir() string { return c.m.LocalOutDir }

// LocalTempDir is dir where temporary files of local tests are written.
func (c *Config) LocalTempDir() string { return c.m.LocalTempDir }

// RemoteRunner is path to executable that runs remote test bundles.
func (c *Config) RemoteRunner() string { return c.m.RemoteRunner }

// RemoteBundleDir is dir where packaged remote test bundles are installed.
func (c *Config) RemoteBundleDir() string { return c.m.RemoteBundleDir }

// PrimaryBundle is the name of the primary bundle that remote fixtures are
// linked to.
func (c *Config) PrimaryBundle() string { return c.m.PrimaryBundle }

// RemoteDataDir is dir containing packaged remote test data.
func (c *Config) RemoteDataDir() string { return c.m.RemoteDataDir }

// RemoteOutDir is dir where intermediate outputs of remote tests are written.
func (c *Config) RemoteOutDir() string { return c.m.RemoteOutDir }

// RemoteTempDir is dir where temporary files of remote tests are written.
func (c *Config) RemoteTempDir() string { return c.m.RemoteTempDir }

// TotalShards is total number of shards to be used in a test run.
func (c *Config) TotalShards() int { return c.m.TotalShards }

// ShardIndex is specifies the index of shard to used in the current run.
func (c *Config) ShardIndex() int { return c.m.ShardIndex }

// SSHRetries is number of SSH connect retries.
func (c *Config) SSHRetries() int { return c.m.SSHRetries }

// ContinueAfterFailure is try to run remaining local tests after bundle/DUT crash or lost SSH connection.
func (c *Config) ContinueAfterFailure() bool { return c.m.ContinueAfterFailure }

// CheckTestDeps is whether test dependencies should be checked.
func (c *Config) CheckTestDeps() bool { return c.m.CheckTestDeps }

// ExcludeSkipped is whether tests which would be skipped are excluded.
func (c *Config) ExcludeSkipped() bool { return c.m.ExcludeSkipped }

// WaitUntilReady is whether to wait for DUT to be ready before running tests.
func (c *Config) WaitUntilReady() bool { return c.m.WaitUntilReady }

// DebuggerPorts is a mapping from binary to the port we want to debug said binary on.
func (c *Config) DebuggerPorts() map[debugger.DebugTarget]int {
	m := make(map[debugger.DebugTarget]int)
	for k, v := range c.m.DebuggerPorts {
		m[k] = v
	}
	return m
}

// ExtraUSEFlags is additional USE flags to inject when determining features.
func (c *Config) ExtraUSEFlags() []string { return append([]string(nil), c.m.ExtraUSEFlags...) }

// Proxy is how proxies should be used.
func (c *Config) Proxy() ProxyMode { return c.m.Proxy }

// CollectSysInfo is collect system info (logs, crashes, etc.) generated during testing.
func (c *Config) CollectSysInfo() bool { return c.m.CollectSysInfo }

// MaxTestFailures is maximum number of test failures.
func (c *Config) MaxTestFailures() int { return c.m.MaxTestFailures }

// TestVars is names and values of variables used to pass out-of-band data to tests.
func (c *Config) TestVars() map[string]string {
	vars := make(map[string]string)
	for k, v := range c.m.TestVars {
		vars[k] = v
	}
	return vars
}

// VarsFiles is paths to variable files.
func (c *Config) VarsFiles() []string { return append([]string(nil), c.m.VarsFiles...) }

// DefaultVarsDirs is dirs containing default variable files.
func (c *Config) DefaultVarsDirs() []string { return append([]string(nil), c.m.DefaultVarsDirs...) }

// MaybeMissingVars is regex matching with variables which may be missing.
func (c *Config) MaybeMissingVars() string { return c.m.MaybeMissingVars }

// MsgTimeout is timeout for reading control messages; default used if zero.
func (c *Config) MsgTimeout() time.Duration { return c.m.MsgTimeout }

// Retries is the number of retries for failing tests
func (c *Config) Retries() int { return c.m.Retries }

// SystemServicesTimeout for waiting for system services to be ready in seconds. (Default: 120 seconds)
func (c *Config) SystemServicesTimeout() time.Duration {
	return c.m.SystemServicesTimeout
}

// DeprecatedState hold state attributes which are accumulated over the course
// of the run.
//
// DEPRECATED: DO NOT add new fields to this struct. DeprecatedState makes it
// difficult to reason about function contracts. Pass arguments explicitly
// instead. This struct will be removed eventually (b/191230756).
type DeprecatedState struct {
	RemoteDevservers []string // list of devserver URLs used by remote tests.
	TestNamesToSkip  []string // tests that match patterns but are not sent to runners to run
}

// NewMutableConfig returns a new configuration for executing test runners in the supplied mode.
// It sets fields that are required by SetFlags.
// tastDir is the base directory under which files are written (e.g. /tmp/tast).
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func NewMutableConfig(mode Mode, tastDir, trunkDir string) *MutableConfig {
	return &MutableConfig{
		Mode:          mode,
		TastDir:       tastDir,
		TrunkDir:      trunkDir,
		TestVars:      make(map[string]string),
		CompanionDUTs: make(map[string]string),
	}
}

// SetFlags adds common run-related flags to f that store values in Config.
func (c *MutableConfig) SetFlags(f *flag.FlagSet) {
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
	f.StringVar(&c.BuildArtifactsURLOverride, "buildartifactsurl", "", "override Google Cloud Storage URL of build artifacts (implies -extraallowedbuckets)")
	f.BoolVar(&c.DownloadPrivateBundles, "downloadprivatebundles", false, "download private bundles if missing")
	ddfs := map[string]int{
		"batch": int(protocol.DownloadMode_BATCH),
		"lazy":  int(protocol.DownloadMode_LAZY),
	}
	ddf := command.NewEnumFlag(ddfs, func(v int) { c.DownloadMode = protocol.DownloadMode(v) }, "batch")
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
	f.StringVar(&c.LocalTempDir, "localtempdir", "/usr/local/tmp/tast/run_tmp", "directory where local test temporary files are written")

	// These are configurable since files may be installed elsewhere when running in the lab.
	f.StringVar(&c.RemoteRunner, "remoterunner", "", "executable that runs remote test bundles")
	f.StringVar(&c.RemoteBundleDir, "remotebundledir", "", "directory containing builtin remote test bundles")
	f.StringVar(&c.RemoteDataDir, "remotedatadir", "", "directory containing builtin remote test data")
	f.StringVar(&c.RemoteTempDir, "remotetempdir", "", "directory where remote test temporary files are written")

	// Both listing and running test requires checking dependency due to sharding.
	// This flag is only used for testing or debugging purpose.
	f.BoolVar(&c.CheckTestDeps, "checktestdeps", true, "skip tests with software dependencies unsatisfied by DUT")

	// Both listing and running test requires filtering and excluding tests that will be
	// skipped. This flag can be used with tast list or tast run to exclude skipped tests
	f.BoolVar(&c.ExcludeSkipped, "excludeskipped", false, "exclude skipped tests from the list or run operation")

	f.Var(command.NewDurationFlag(time.Second, &c.SystemServicesTimeout, defaultSystemServicesTimeout), "systemservicestimeout", "timeout for waiting for system services to be ready in seconds")

	c.DebuggerPorts = map[debugger.DebugTarget]int{
		debugger.LocalBundle:  0,
		debugger.RemoteBundle: 0,
	}
	debuggerFlag := command.RepeatedFlag(func(v string) error {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			return errors.New("attachdebugger flag must take the form 'local:2345', 'remote:2346', or similar")
		}
		target := debugger.DebugTarget(parts[0])
		if _, ok := c.DebuggerPorts[target]; !ok {
			return fmt.Errorf("unknown debug target '%s' - valid targets are %s and %s", target, debugger.LocalBundle, debugger.RemoteBundle)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return errors.New("attachdebugger flag must take the form 'local:2345', 'remote:2346', or similar")
		}
		c.DebuggerPorts[target] = port
		return nil
	})
	f.Var(&debuggerFlag, "attachdebugger", "start up the delve debugger for a process and wait for a process to attach on a given port")

	// Some flags are only relevant if we're running tests rather than listing them.
	if c.Mode == RunTestsMode {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
		f.BoolVar(&c.CollectSysInfo, "sysinfo", true, "collect system information (logs, crashes, etc.)")
		f.BoolVar(&c.WaitUntilReady, "waituntilready", true, "wait until DUT is ready before running tests")

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
		// TODO(oka): Use flag.Func once it's available.
		f.Var(funcValue(func(s string) error {
			c.MaybeMissingVars = s
			_, err := regexp.Compile(s)
			return err
		}), "maybemissingvars", "Regex matching with variables which may be missing")

		vals := map[string]int{
			"env":  int(ProxyEnv),
			"none": int(ProxyNone),
		}
		td := command.NewEnumFlag(vals, func(v int) { c.Proxy = ProxyMode(v) }, "env")
		desc := fmt.Sprintf("proxy settings used by the DUT (%s; default %q)", td.QuotedValues(), td.Default())
		f.Var(td, "proxy", desc)

		compDUTs := command.RepeatedFlag(func(roleToDUT string) error {
			parts := strings.SplitN(roleToDUT, ":", 2)
			if len(parts) != 2 {
				return errors.New(`want "role:address"`)
			}
			c.CompanionDUTs[parts[0]] = parts[1]
			return nil
		})
		f.Var(&compDUTs, "companiondut", `role to companion DUT, as "role:address" (can be repeated)`)
		f.IntVar(&c.Retries, "retries", 0, `number of times to retry a failing test`)

	}
}

type funcValue func(string) error // implements flag.Value

func (f funcValue) Set(s string) error { return f(s) }
func (f funcValue) String() string     { return "" }

// DeriveDefaults sets default config values to unset members, possibly deriving from
// already set members. It should be called after non-default values are set to c.
func (c *MutableConfig) DeriveDefaults() error {
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
	} else {
		// If -build=false, default values are paths to files installed by Portage.
		setIfEmpty(&c.LocalRunner, "/usr/local/bin/local_test_runner")
		setIfEmpty(&c.LocalBundleDir, "/usr/local/libexec/tast/bundles/local")
		setIfEmpty(&c.LocalDataDir, "/usr/local/share/tast/data")
		setIfEmpty(&c.RemoteRunner, "/usr/bin/remote_test_runner")
		setIfEmpty(&c.RemoteBundleDir, "/usr/libexec/tast/bundles/remote")
		setIfEmpty(&c.RemoteDataDir, "/usr/share/tast/data")
	}
	// TODO(crbug/1177189): we assume there's only one remote bundle. Consider
	// removing the restriction.
	c.PrimaryBundle = "cros"

	c.MsgTimeout = time.Minute

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

	if c.BuildArtifactsURLOverride != "" {
		if !strings.HasSuffix(c.BuildArtifactsURLOverride, "/") {
			return errors.New("-buildartifactsurl must end with a slash")
		}

		// Add the bucket to the extra allowed bucket list.
		u, err := url.Parse(c.BuildArtifactsURLOverride)
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

	if !c.Build {
		for _, port := range c.DebuggerPorts {
			if port != 0 {
				return errors.New("-build=false and -attachdebugger are mutually exclusive (you can't attach the debugger to something that wasn't built with debugging symbols)")
			}
		}
	}

	return nil
}

// Freeze returns a frozen configuration object.
func (c *MutableConfig) Freeze() *Config {
	return &Config{m: c}
}

// BuildCfg returns a build.Config.
func (c *Config) BuildCfg() *build.Config {
	return &build.Config{
		CheckBuildDeps:     c.CheckPortageDeps(),
		CheckDepsCachePath: filepath.Join(c.BuildOutDir(), checkDepsCacheFile),
		InstallPortageDeps: c.InstallPortageDeps(),
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
	return filepath.Join(c.TrunkDir(), "src/platform/tast")
}

// crosTestWorkspace returns the Go workspace containing standard test-related code.
// This workspace also contains the default "cros" test bundles.
func (c *Config) crosTestWorkspace() string {
	return filepath.Join(c.TrunkDir(), "src/platform/tast-tests")
}

// BundleWorkspaces returns Go workspaces containing source code needed to build c.BuildBundle.
func (c *Config) BundleWorkspaces() []string {
	ws := []string{c.crosTestWorkspace()}
	ws = append(ws, c.CommonWorkspaces()...)

	// If a custom test bundle workspace was specified, prepend it.
	if c.BuildWorkspace() != ws[0] {
		ws = append([]string{c.BuildWorkspace()}, ws...)
	}
	return ws
}

// LocalBundleGlob returns a file path glob that matches local test bundle executables.
func (c *Config) LocalBundleGlob() string {
	return c.bundleGlob(c.LocalBundleDir())
}

// RemoteBundleGlob returns a file path glob that matches remote test bundle executables.
func (c *Config) RemoteBundleGlob() string {
	return c.bundleGlob(c.RemoteBundleDir())
}

// Features constructs Features from Config and protocol.DUTFeatures.
func (c *Config) Features(dut *protocol.DUTFeatures) *protocol.Features {
	return &protocol.Features{
		CheckDeps: c.CheckTestDeps(),
		Infra: &protocol.InfraFeatures{
			Vars:             c.TestVars(),
			MaybeMissingVars: c.MaybeMissingVars(),
		},
		Dut: dut,
	}
}

func (c *Config) bundleGlob(dir string) string {
	last := "*"
	if c.Build() {
		last = c.BuildBundle()
	}
	return filepath.Join(dir, last)
}
