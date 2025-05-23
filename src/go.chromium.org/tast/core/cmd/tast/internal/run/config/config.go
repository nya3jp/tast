// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package config defines common structs to carry configuration and associated
// stateful data.
package config

import (
	"bufio"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/tast/core/cmd/tast/internal/build"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/command"
	"go.chromium.org/tast/core/internal/debugger"
	"go.chromium.org/tast/core/internal/protocol"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

// Mode describes the action to perform.
type Mode int

const (
	// RunTestsMode indicates that tests should be run and their results reported.
	RunTestsMode Mode = iota
	// ListTestsMode indicates that tests should only be listed.
	ListTestsMode
	// GlobalRuntimeVarsMode indicates that list all GlobalRuntimeVars currently used.
	GlobalRuntimeVarsMode
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
	defaultKeyFile               = "chromite/ssh_keys/testing_rsa" // default private SSH key within ChromeOS checkout
	checkDepsCacheFile           = "check_deps_cache.v2.json"      // file in BuildOutDir where dependency-checking results are cached
	defaultSystemServicesTimeout = 120 * time.Second               // default timeout for waiting for system services to be ready in seconds
	defaultMsgTimeout            = 120 * time.Second               // default timeout for grpc connection.
	defaultWaitUntilReadyTimeout = 120 * time.Second               // default timeout for the entire ready.Wait function
	dutNotToConnect              = "-"                             // Target for dutless scenarios
	defaultMaxSysMsgLogSize      = 20 * 1024 * 1024                // default Max System Message Log Size 20MB
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
	SwarmingTaskID            string
	BuildBucketID             string
	DUTLabConfig              *frameworkprotocol.DUTLabConfig

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
	ShardMethod string

	SSHRetries           int
	ContinueAfterFailure bool
	CheckTestDeps        bool
	WaitUntilReady       bool
	ExtraUSEFlags        []string
	Proxy                ProxyMode
	CollectSysInfo       bool
	MaxTestFailures      int
	ExcludeSkipped       bool
	ProxyCommand         string

	TestVars         map[string]string
	VarsFiles        []string
	DefaultVarsDirs  []string
	MaybeMissingVars string

	DebuggerPorts          map[debugger.DebugTarget]int
	DebuggerPortForwarding bool

	Retries int
	Repeats int

	SystemServicesTimeout time.Duration
	MsgTimeout            time.Duration
	WaitUntilReadyTimeout time.Duration
	MaxSysMsgLogSize      int64

	// ForceSkips is a mapping from a test name to the filter file name which specified
	// the test should be disabled.
	// Filter file example:
	//     # The following tests will disabled.
	//     -meta.DisabledTest1
	//     -meta.DisabledTest2
	ForceSkips map[string]*protocol.ForceSkip

	DefaultHardwareFeatures *frameworkprotocol.HardwareFeatures
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
		ProxyCommand:   c.ProxyCommand(),
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

// TrunkDir is path to ChromeOS checkout.
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

// DUTLabConfig specifies the DUT lab configuration of the DUTs
// used for the running Tast.
func (c *Config) DUTLabConfig() *frameworkprotocol.DUTLabConfig {
	return c.m.DUTLabConfig
}

// SwarmingTaskID specifies the swarming task ID of the scheduled
// job that run Tast tests.
func (c *Config) SwarmingTaskID() string { return c.m.SwarmingTaskID }

// BuildBucketID specifies the build bucket ID of the scheduled
// job that run Tast tests.
func (c *Config) BuildBucketID() string { return c.m.BuildBucketID }

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

// ShardIndex specifies the index of shard to used in the current run.
func (c *Config) ShardIndex() int { return c.m.ShardIndex }

// ShardMethod specifies which sharding method we should use.
func (c *Config) ShardMethod() string { return c.m.ShardMethod }

// SSHRetries is number of SSH connect retries.
func (c *Config) SSHRetries() int { return c.m.SSHRetries }

// ContinueAfterFailure is try to run remaining local tests after bundle/DUT crash or lost SSH connection.
func (c *Config) ContinueAfterFailure() bool { return c.m.ContinueAfterFailure }

// CheckTestDeps is whether test dependencies should be checked.
func (c *Config) CheckTestDeps() bool { return c.m.CheckTestDeps }

// ExcludeSkipped is whether tests which would be skipped are excluded.
func (c *Config) ExcludeSkipped() bool { return c.m.ExcludeSkipped }

// ProxyCommand specifies the command to use to connect to the DUT.
func (c *Config) ProxyCommand() string { return c.m.ProxyCommand }

// WaitUntilReady is whether to wait for DUT to be ready before running tests.
func (c *Config) WaitUntilReady() bool { return c.m.WaitUntilReady }

// WaitUntilReadyTimeout set a timeout for the entire ready.Wait function. (Default: 120 seconds)
func (c *Config) WaitUntilReadyTimeout() time.Duration {
	return c.m.WaitUntilReadyTimeout
}

// DebuggerPorts is a mapping from binary to the port we want to debug said binary on.
func (c *Config) DebuggerPorts() map[debugger.DebugTarget]int {
	m := make(map[debugger.DebugTarget]int)
	for k, v := range c.m.DebuggerPorts {
		m[k] = v
	}
	return m
}

// DebuggerPortForwarding is whether port forwarding should be performed for you when debugging.
func (c *Config) DebuggerPortForwarding() bool { return c.m.DebuggerPortForwarding }

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

// MaxSysMsgLogSize is size for truncate log files.
func (c *Config) MaxSysMsgLogSize() int64 { return c.m.MaxSysMsgLogSize }

// Retries is the number of retries for failing tests
func (c *Config) Retries() int { return c.m.Retries }

// Repeats is the number of times each subsequent test should execute.
func (c *Config) Repeats() int { return c.m.Repeats }

// SystemServicesTimeout for waiting for system services to be ready in seconds. (Default: 120 seconds)
func (c *Config) SystemServicesTimeout() time.Duration {
	return c.m.SystemServicesTimeout
}

// ForceSkips returns the mapping between name of disabled tests
// and the reason of disabling the test.
func (c *Config) ForceSkips() map[string]*protocol.ForceSkip {
	forceSkips := make(map[string]*protocol.ForceSkip)
	for k, r := range c.m.ForceSkips {
		forceSkips[k] = &protocol.ForceSkip{Reason: r.Reason}
	}
	return forceSkips
}

// DefaultHardwareFeatures returns the hardware features to use if ssh is disabled.
func (c *Config) DefaultHardwareFeatures() *frameworkprotocol.HardwareFeatures {
	return c.m.DefaultHardwareFeatures
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
// trunkDir is the path to the ChromeOS checkout (within the chroot).
func NewMutableConfig(mode Mode, tastDir, trunkDir string) *MutableConfig {
	return &MutableConfig{
		Mode:          mode,
		TastDir:       tastDir,
		TrunkDir:      trunkDir,
		TestVars:      make(map[string]string),
		CompanionDUTs: make(map[string]string),
		ForceSkips:    make(map[string]*protocol.ForceSkip),
	}
}

// ShouldConnect tells whether we should connect to the specified target.
func ShouldConnect(target string) bool {
	return target != dutNotToConnect
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
	ddf := command.NewEnumFlag(ddfs, func(v int) { c.DownloadMode = protocol.DownloadMode(v) }, "lazy")
	f.Var(ddf, "downloaddata", fmt.Sprintf("strategy to download external data files (%s; default %q)", ddf.QuotedValues(), ddf.Default()))
	f.BoolVar(&c.ContinueAfterFailure, "continueafterfailure", true, "try to run remaining tests after bundle/DUT crash or lost SSH connection")
	f.IntVar(&c.SSHRetries, "sshretries", 0, "number of SSH connect retries")
	f.StringVar(&c.TLWServer, "tlwserver", "", "TLW server address")
	f.StringVar(&c.ReportsServer, "reports_server", "", "Reports server address")
	f.IntVar(&c.MaxTestFailures, "maxtestfailures", 0, "the maximum number test failures allowed (default to 0 which means no limit)")
	f.StringVar(&c.ProxyCommand, "proxycommand", "", "command to use to connect to the DUT.")

	f.IntVar(&c.TotalShards, "totalshards", 1, "total number of shards to be used in a test run")
	f.IntVar(&c.ShardIndex, "shardindex", 0, "the index of shard to used in the current run")
	f.StringVar(&c.ShardMethod, "shardmethod", "alpha", "the method used to split the shards (one of \"hash\" or \"alpha\")")

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
	f.Var(command.NewDurationFlag(time.Second, &c.MsgTimeout, defaultMsgTimeout), "connectiontimeout", "the value time interval in seconds for tast to check if the connection to target is alive (default to 60 which means 1 mins)")
	f.Int64Var(&c.MaxSysMsgLogSize, "maxsysmsglogsize", defaultMaxSysMsgLogSize, "max size for the downloaded system message log after each test (default to 20MB)")

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
	f.BoolVar(&c.DebuggerPortForwarding, "debuggerportforwarding", true, "Forward ports for you when attempting to connect to a dlv instance on a DUT. If set to false, you will need to forward ports yourself (ssh -R port:localhost:port).")

	filterFile := command.RepeatedFlag(func(fileName string) error {
		if err := c.addForceSkippedTests(fileName); err != nil {
			return errors.Wrapf(err, "failed to read test filter file %s", fileName)
		}
		return nil
	})
	f.Var(&filterFile, "testfilterfile", `a file indicates which tests to be disable (can be repeated)`)

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
	f.Var(funcValue(func(s string) error {
		var hwFeatures frameworkprotocol.HardwareFeatures
		err := prototext.Unmarshal([]byte(s), &hwFeatures)
		if err != nil {
			return err
		}
		c.DefaultHardwareFeatures = &hwFeatures
		return nil
	}), "hwdeps", "Set HardwareFeatures proto for duts without ssh")

	// Some flags are only relevant if we're running tests rather than listing them.
	if c.Mode == RunTestsMode {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
		f.BoolVar(&c.CollectSysInfo, "sysinfo", true, "collect system information (logs, crashes, etc.)")
		f.BoolVar(&c.WaitUntilReady, "waituntilready", true, "wait until DUT is ready before running tests")
		f.Var(command.NewDurationFlag(time.Second, &c.WaitUntilReadyTimeout, defaultWaitUntilReadyTimeout), "waituntilreadytimeout", "timeout for the entire ready.Wait function")

		f.Var(command.NewListFlag(",", func(v []string) { c.ExtraUSEFlags = v }, nil), "extrauseflags",
			"comma-separated list of additional USE flags to inject when checking test dependencies")

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

		readLabConfig := func(filename string) error {
			labConfig := &frameworkprotocol.DUTLabConfig{}
			data, err := os.ReadFile(filename)
			if err != nil {
				return errors.Wrapf(err, "failed to open lab config fiie %s", filename)
			}
			unmarshaler := protojson.UnmarshalOptions{
				DiscardUnknown: true,
			}
			if err := unmarshaler.Unmarshal(data, labConfig); err != nil {
				return errors.Wrapf(err, "failed to unmarshal lab config fiie %s", filename)
			}
			c.DUTLabConfig = labConfig
			return nil
		}
		f.Func("dutlabconfig", `a file to describe all DUTs being used in the test`,
			readLabConfig)

		f.IntVar(&c.Retries, "retries", 0, `number of times to retry a failing test`)
		f.IntVar(&c.Repeats, "repeats", 0, `number of times to execute a set of tests after the initial execution`)
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
		setIfEmpty(&c.RemoteDataDir, "/usr/share/tast/data")
		if !InChroot() || c.DownloadPrivateBundles {
			setIfEmpty(&c.RemoteBundleDir, "/usr/libexec/tast/bundles/remote")
		} else {
			setIfEmpty(&c.RemoteBundleDir, filepath.Join(c.BuildOutDir, build.ArchHost, build.RemoteBundleBuildSubdir))
		}

	}
	// TODO(crbug/1177189): we assume there's only one remote bundle. Consider
	// removing the restriction.
	c.PrimaryBundle = "cros"

	// Apply -varsfile.
	for _, path := range c.VarsFiles {
		if err := readAndMergeVarsFile(c.TestVars, path, errorOnDuplicate); err != nil {
			return fmt.Errorf("failed to apply vars from %s: %v", path, err)
		}
	}

	// Apply variables from default configurations.
	if len(c.DefaultVarsDirs) == 0 {
		// TODO: b/324133828 -- Use only src/* after /etc/tast/vars are removed
		// from the build.
		if c.Build {
			c.DefaultVarsDirs = []string{
				filepath.Join(c.TrunkDir, "src/platform/tast-tests-private/vars"),
				filepath.Join(c.TrunkDir, "src/platform/tast-tests/vars"),
			}
		} else {
			privateVars := filepath.Join(c.TrunkDir, "src/platform/tast-tests-private/vars")
			if _, err := os.Stat("/etc/tast/vars/private"); err == nil {
				privateVars = "/etc/tast/vars/private"
			}
			publicVars := filepath.Join(c.TrunkDir, "src/platform/tast-tests/vars")
			if _, err := os.Stat("/etc/tast/vars/public"); err == nil {
				publicVars = "/etc/tast/vars/public"
			}
			c.DefaultVarsDirs = []string{privateVars, publicVars}
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
	if c.ShardMethod != "alpha" && c.ShardMethod != "hash" {
		return fmt.Errorf("-shardmethod must be either 'hash' or 'alpha'")
	}

	if !c.Build {
		for _, port := range c.DebuggerPorts {
			if port != 0 {
				return errors.New("-build=false and -attachdebugger are mutually exclusive (you can't attach the debugger to something that wasn't built with debugging symbols)")
			}
		}
	}

	c.SwarmingTaskID = os.Getenv("SWARMING_TASK_ID")
	c.BuildBucketID = os.Getenv("BUILD_BUCKET_ID")

	return nil
}

// Freeze returns a frozen configuration object.
func (c *MutableConfig) Freeze() *Config {
	return &Config{m: c}
}

// addForceSkippedTests extracts name of disabled test name from a filter
// file and put the information to the c.ForceSkips which is a mapping
// from a test name to the reason being disabled.
// Filter file example:
//
//	# The following tests will disabled.
//	-meta.DisabledTest1
//	-meta.DisabledTest2
func (c *MutableConfig) addForceSkippedTests(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return errors.Wrapf(err, "failed to read test filter file: %v", fileName)
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := sc.Text()
		tokens := strings.Split(line, "#")
		testName := strings.TrimSpace(tokens[0])
		if strings.HasPrefix(testName, "-") {
			t := testName[1:]
			c.ForceSkips[t] = &protocol.ForceSkip{
				Reason: fmt.Sprintf("Test %s is disabled by test filter file %s", t, fileName),
			}
		} else if testName != "" {
			return errors.Wrapf(err, "filter file %v has syntax error: %q", fileName, line)
		}
	}
	return nil
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
func (c *Config) Features(dut *frameworkprotocol.DUTFeatures,
	companions map[string]*frameworkprotocol.DUTFeatures) *protocol.Features {
	return &protocol.Features{
		CheckDeps: c.CheckTestDeps(),
		Infra: &protocol.InfraFeatures{
			Vars:             c.TestVars(),
			MaybeMissingVars: c.MaybeMissingVars(),
			DUTLabConfig:     c.DUTLabConfig(),
		},
		Dut:               dut,
		CompanionFeatures: companions,
		ForceSkips:        c.ForceSkips(),
	}
}

func (c *Config) bundleGlob(dir string) string {
	last := "*"
	if c.Build() {
		last = c.BuildBundle()
	}
	return filepath.Join(dir, last)
}

// InChroot checks if the current session is running inside chroot.
func InChroot() bool {
	if _, err := os.Stat("/etc/cros_chroot_version"); err == nil {
		return true
	}
	return false
}
