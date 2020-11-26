// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/testing"
)

// RunMode describes the runner's behavior.
type RunMode int

const (
	// RunTestsMode indicates that the runner should run all matched tests.
	RunTestsMode RunMode = 0
	// ListTestsMode indicates that the runner should write information about matched tests to stdout as a
	// JSON array of testing.Test structs and exit.
	ListTestsMode = 2
	// GetSysInfoStateMode indicates that the runner should write a JSON-marshaled GetSysInfoStateResult struct
	// to stdout and exit. It's used by the tast executable to get the initial state of the system before tests
	// are executed. This mode is only supported by local_test_runner.
	GetSysInfoStateMode = 3
	// CollectSysInfoMode indicates that the runner should collect system information that was written in the
	// course of testing and write a JSON-marshaled CollectSysInfoResult struct to stdout and exit. It's used by
	// the tast executable to get system info after testing is completed.
	// This mode is only supported by local_test_runner.
	CollectSysInfoMode = 4
	// GetDUTInfoMode indicates that the runner should return DUT information via a JSON-marshaled
	// GetDUTInfoResult struct written to stdout. This mode is only supported by local_test_runner.
	GetDUTInfoMode = 5
	// DownloadPrivateBundlesMode indicates that the runner should download private bundles from devservers,
	// install them to the DUT, write a JSON-marshaled DownloadPrivateBundlesResult struct to stdout and exit.
	// This mode is only supported by local_test_runner.
	DownloadPrivateBundlesMode = 6
	// ListFixturesMode indicates that the runner should write information about fixtures to stdout
	// as a JSON serialized ListFixturesResult.
	ListFixturesMode = 7
)

// Args provides a backward- and forward-compatible way to pass arguments from the tast executable to test runners.
// The tast executable writes the struct's JSON-serialized representation to the runner's stdin.
type Args struct {
	// Mode describes the mode that should be used by the runner.
	Mode RunMode `json:"mode"`

	// RunTests contains arguments used by RunTestsMode.
	RunTests *RunTestsArgs `json:"runTests,omitempty"`
	// ListTests contains arguments used by ListTestsMode.
	ListTests *ListTestsArgs `json:"listTests,omitempty"`
	// ListFixtures contains arguments used by ListFixturesMode.
	ListFixtures *ListFixturesArgs `json:"listFixtures,omitempty"`
	// CollectSysInfo contains arguments used by CollectSysInfoMode.
	CollectSysInfo *CollectSysInfoArgs `json:"collectSysInfo,omitempty"`
	// GetDUTInfo contains arguments used by GetDUTInfoMode.
	// Note that, for backward compatibility, the JSON's field name is getSoftwareFeatures.
	GetDUTInfo *GetDUTInfoArgs `json:"getSoftwareFeatures,omitempty"`
	// DownloadPrivateBundles contains arguments used by DownloadPrivateBundlesMode.
	DownloadPrivateBundles *DownloadPrivateBundlesArgs `json:"downloadPrivateBundles,omitempty"`

	// report is set to true by readArgs if status should be reported via control messages rather
	// than human-readable log messages. This is true when args were supplied via stdin rather than
	// command-line flags, indicating that the runner was executed by the tast command. It's only relevant
	// for RunTestsMode.
	report bool
}

// bundleArgs creates a bundle.Args appropriate for running bundles in the supplied mode.
// The returned struct's slices should not be modified, as they are shared with a.
func (a *Args) bundleArgs(mode bundle.RunMode) (*bundle.Args, error) {
	ba := bundle.Args{Mode: mode}

	switch mode {
	case bundle.RunTestsMode:
		switch a.Mode {
		case RunTestsMode:
			ba.RunTests = &a.RunTests.BundleArgs
		default:
			return nil, fmt.Errorf("can't make RunTests bundle args in runner mode %d", int(a.Mode))
		}
	case bundle.ListTestsMode:
		switch a.Mode {
		case RunTestsMode:
			// We didn't receive ListTests args, so copy the shared patterns field from RunTests.
			ba.ListTests = &bundle.ListTestsArgs{Patterns: a.RunTests.BundleArgs.Patterns}
		case ListTestsMode:
			ba.ListTests = &a.ListTests.BundleArgs
		default:
			return nil, fmt.Errorf("can't make ListTests bundle args in runner mode %d", int(a.Mode))
		}
	}

	// Backfill deprecated fields in case we're executing an old test bundle.
	ba.FillDeprecated()

	return &ba, nil
}

// fillDefaults fills unset fields with default values from cfg.
func (a *Args) fillDefaults(cfg *Config) {
	switch a.Mode {
	case RunTestsMode:
		if a.RunTests.BundleArgs.BuildArtifactsURL == "" {
			a.RunTests.BundleArgs.BuildArtifactsURL = cfg.DefaultBuildArtifactsURL
		}
	case DownloadPrivateBundlesMode:
		if a.DownloadPrivateBundles.BuildArtifactsURL == "" {
			a.DownloadPrivateBundles.BuildArtifactsURL = cfg.DefaultBuildArtifactsURL
		}
	}
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by the tast process to ensure that args will be interpreted
// correctly by older test runners.
func (a *Args) FillDeprecated() {
	// If there were any deprecated fields, we would fill them from the corresponding
	// non-deprecated fields here using command.CopyFieldIfNonZero for basic types or
	// manual copies for structs.
	if a.RunTests != nil {
		command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.BuildArtifactsURL, &a.RunTests.BuildArtifactsURLDeprecated)
	}
}

// PromoteDeprecated copies all non-zero-valued deprecated fields to the corresponding non-deprecated fields.
// Missing sub-structs (e.g. RunTestsArgs and ListTestsArgs) are initialized.
// This method is called by test runners to normalize args that were marshaled by an older tast executable.
//
// If both an old and new field are set, the old field takes precedence. This is counter-intuitive but
// necessary: a default value for the new field may have been passed to Run. If the corresponding old field
// is non-zero, it was passed by an old tast executable (or by a new executable that called FillDeprecated),
// so we use the old field to make sure that it overrides the default.
func (a *Args) PromoteDeprecated() {
	if a.RunTests != nil {
		command.CopyFieldIfNonZero(&a.RunTests.BuildArtifactsURLDeprecated, &a.RunTests.BundleArgs.BuildArtifactsURL)
	}
}

// RunTestsArgs is nested within Args and contains arguments used by RunTestsMode.
type RunTestsArgs struct {
	// BundleArgs contains arguments that are relevant to test bundles.
	BundleArgs bundle.RunTestsArgs `json:"bundleArgs"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`
	// BuildArtifactsURLDeprecated is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	// If it is empty, DefaultBuildArtifactsURL in runner.Config is used.
	// DEPRECATED: Use bundle.RunTestsArgs.BuildArtifactsURL instead.
	BuildArtifactsURLDeprecated string `json:"buildArtifactsUrl,omitempty"`
}

// ListTestsArgs is nested within Args and contains arguments used by ListTestsMode.
type ListTestsArgs struct {
	// BundleArgs contains arguments that are relevant to test bundles.
	BundleArgs bundle.ListTestsArgs `json:"bundleArgs"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
}

// ListFixturesArgs is nested within Args and contains arguments used by ListFixturesMode.
type ListFixturesArgs struct {
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
}

// ListFixturesResult holds the result of a ListFixturesMode command.
type ListFixturesResult struct {
	// Fixtures maps bundle path to the fixtures it contains.
	Fixtures map[string][]*testing.EntityInfo `json:"fixtures,omitempty"`
}

// GetSysInfoStateResult holds the result of a GetSysInfoStateMode command.
type GetSysInfoStateResult struct {
	// SysInfoState contains the collected state.
	State SysInfoState `json:"state"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
	// Each warning can be logged directly without additional information.
	Warnings []string `json:"warnings,omitempty"`
}

// CollectSysInfoArgs is nested within Args and holds arguments used by CollectSysInfoMode.
type CollectSysInfoArgs struct {
	// InitialState describes the pre-testing state of the DUT. It should be generated by a GetSysInfoStateMode
	// command executed before tests are run.
	InitialState SysInfoState `json:"initialState"`
}

// CollectSysInfoResult contains the result of a CollectSysInfoMode command.
type CollectSysInfoResult struct {
	// LogDir is the directory where log files were copied. The caller should delete it.
	LogDir string `json:"logDir,omitempty"`
	// CrashDir is the directory where minidump crash files were copied. The caller should delete it.
	CrashDir string `json:"crashDir,omitempty"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
	Warnings []string `json:"warnings,omitempty"`
}

// GetDUTInfoArgs is nested within Args and contains arguments used by GetDUTInfoMode.
type GetDUTInfoArgs struct {
	// ExtraUSEFlags lists USE flags that should be treated as being set an addition to
	// the ones read from Config.USEFlagsFile when computing the feature sets for GetDUTInfoResult.
	ExtraUSEFlags []string `json:"extraUseFlags,omitempty"`

	// RequestDeviceConfig specifies if GetDUTInfoMode should return a device.Config instance
	// generated from runtime DUT configuration.
	RequestDeviceConfig bool `json:"requestDeviceConfig,omitempty"`
}

// GetDUTInfoResult contains the result of a GetDUTInfoMode command.
type GetDUTInfoResult struct {
	// SoftwareFeatures contains the information about the software features of the DUT.
	// For backward compatibility, in JSON format, fields are flatten.
	// This struct has MarshalJSON/UnmarshalJSON and the serialization/deserialization
	// of this field are handled in the methods respectively.
	SoftwareFeatures *dep.SoftwareFeatures `json:"-"`

	// DeviceConfig contains the DUT's device characteristic.
	// Similar to SoftwareFeatures field, the serialization/deserialization
	// of this field are handled in MarshalJSON/UnmarshalJSON respectively.
	DeviceConfig     *device.Config             `json:"-"`
	HardwareFeatures *configpb.HardwareFeatures `json:"-"`

	// OSVersion contains the DUT's OS Version
	OSVersion string `json:"osVersion,omitempty"`

	// Warnings contains descriptions of non-fatal errors encountered while determining features.
	Warnings []string `json:"warnings,omitempty"`
}

// MarshalJSON marshals the given GetDUTInfoResult with handing protocol
// backward compatibility.
func (r *GetDUTInfoResult) MarshalJSON() ([]byte, error) {
	var available, missing []string
	if r.SoftwareFeatures != nil {
		available = r.SoftwareFeatures.Available
		missing = r.SoftwareFeatures.Unavailable
	}

	var dc []byte
	if r.DeviceConfig != nil {
		var err error
		dc, err = proto.Marshal(r.DeviceConfig)
		if err != nil {
			return nil, err
		}
	}
	var hf []byte
	if r.HardwareFeatures != nil {
		var err error
		hf, err = proto.Marshal(r.HardwareFeatures)
		if err != nil {
			return nil, err
		}
	}

	type Alias GetDUTInfoResult
	return json.Marshal(struct {
		Available        []string `json:"available,omitempty"`
		Missing          []string `json:"missing,omitempty"`
		DeviceConfig     []byte   `json:"deviceConfig,omitempty"`
		HardwareFeatures []byte   `json:"hardwareFeatures,omitempty"`
		*Alias
	}{
		Available:        available,
		Missing:          missing,
		DeviceConfig:     dc,
		HardwareFeatures: hf,
		Alias:            (*Alias)(r),
	})
}

// UnmarshalJSON unmarshals the given b to this r object with handing protocol
// backward compatibility.
func (r *GetDUTInfoResult) UnmarshalJSON(b []byte) error {
	type Alias GetDUTInfoResult
	aux := struct {
		Available        []string `json:"available,omitempty"`
		Missing          []string `json:"missing,omitempty"`
		DeviceConfig     []byte   `json:"deviceConfig,omitempty"`
		HardwareFeatures []byte   `json:"hardwareFeatures,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if len(aux.Available) > 0 || len(aux.Missing) > 0 {
		r.SoftwareFeatures = &dep.SoftwareFeatures{
			Available:   aux.Available,
			Unavailable: aux.Missing,
		}
	}
	if len(aux.DeviceConfig) > 0 {
		var dc device.Config
		if err := proto.Unmarshal(aux.DeviceConfig, &dc); err != nil {
			return err
		}
		r.DeviceConfig = &dc
	}
	if len(aux.HardwareFeatures) > 0 {
		var hf configpb.HardwareFeatures
		if err := proto.Unmarshal(aux.HardwareFeatures, &hf); err != nil {
			return err
		}
		r.HardwareFeatures = &hf
	}
	return nil
}

// SysInfoState contains the state of the DUT's system information.
type SysInfoState struct {
	// LogInodeSizes maps from each log file's inode to its size in bytes.
	LogInodeSizes map[uint64]int64 `json:"logInodeSizes,omitempty"`
	// UnifiedLogCursor contains an opaque cursor pointing at the current tip of unified system logs.
	// The name of json field is "journaldCursor" for historical reason.
	UnifiedLogCursor string `json:"journaldCursor,omitempty"`
	// MinidumpPaths contains absolute paths to minidump crash files.
	MinidumpPaths []string `json:"minidumpPaths,omitempty"`
}

// DownloadPrivateBundlesArgs is nested within Args and contains arguments used by DownloadPrivateBundlesMode.
type DownloadPrivateBundlesArgs struct {
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`

	// TLWServer contains host and port name of TLW server that can be used for downloading files.
	// When this is set, it takes precedence over Devservers.
	TLWServer string `json:"tlsServer,omitempty"`

	// DUTName contains the name of the DUT recognized by the TLW service.
	// This must be set when TLWServer is used.
	DUTName string `json:"dutName,omitempty"`

	// BuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	// If it is empty, DefaultBuildArtifactsURL in runner.Config is used.
	BuildArtifactsURL string `json:"buildArtifactsUrl,omitempty"`
}

// DownloadPrivateBundlesResult contains the result of a DownloadPrivateBundlesMode command.
type DownloadPrivateBundlesResult struct {
	// Messages contains log messages emitted while downloading test bundles.
	Messages []string `json:"logs,omitempty"`
}

// RunnerType describes the type of test runner that is using this package.
type RunnerType int // NOLINT

const (
	// LocalRunner indicates that this package is being used by local_test_runner.
	LocalRunner RunnerType = iota
	// RemoteRunner indicates that this package is being used by remote_test_runner.
	RemoteRunner
)

// Config contains fixed parameters for the runner that are passed in from local_test_runner
// or remote_test_runner.
type Config struct {
	// Type describes the type of runner being executed.
	Type RunnerType

	// KillStaleRunners dictates whether SIGTERM should be sent to any existing test runner processes
	// when using RunTestsMode. This can help prevent confusing failures if multiple test jobs are
	// incorrectly scheduled on the same DUT: https://crbug.com/941829
	KillStaleRunners bool

	// SystemLogDir contains the directory where information is logged by syslog and other daemons.
	SystemLogDir string
	// SystemLogExcludes contains relative paths of directories and files in SystemLogDir to exclude.
	SystemLogExcludes []string
	// UnifiedLogSubdir contains the subdirectory within CollectSysInfoResult.LogDir where unified system logs will be written.
	// No system logs will be be collected if this is empty.
	UnifiedLogSubdir string `json:"-"`
	// SystemInfoFunc contains a function that will be executed to gather additional system info.
	// The information should be written to dir.
	SystemInfoFunc func(ctx context.Context, dir string) error
	// SystemCrashDirs contains directories where crash dumps are written when processes crash.
	SystemCrashDirs []string
	// CleanupLogsPausedPath is a path to the marker file on the DUT to pause log cleanup.
	CleanupLogsPausedPath string

	// USEFlagsFile contains the path to a file listing a subset of USE flags that were set when building
	// the system image. These USE flags are used by expressions in SoftwareFeatureDefinitions to determine
	// available software features.
	USEFlagsFile string
	// LSBReleaseFile contains the path to the lsb-release file to determine board name used for
	// the expressions in SoftwareFeatureDefinitions.
	LSBReleaseFile string
	// SoftwareFeatureDefinitions maps from software feature names (e.g. "myfeature") to boolean expressions
	// used to compose them from USE flags (e.g. "a && !(b || c)"). The USE flags used in these expressions
	// must be listed in USEFlagsFile if they were set when building the system image.
	// See chromiumos/tast/internal/expr for details about expression syntax.
	SoftwareFeatureDefinitions map[string]string
	// AutotestCapabilityDir contains the path to a directory containing autotest-capability YAML files used to
	// define the DUT's capabilities for the purpose of determining which video tests it is able to run.
	// See https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/autotest-capability-default/
	// and the autocaps package for more information.
	AutotestCapabilityDir string
	// DefaultBuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image. It can be empty if the image is
	// not built by an official builder.
	DefaultBuildArtifactsURL string
	// PrivateBundlesStampPath contains the path to a stamp file indicating private test bundles have been
	// successfully downloaded and installed before. This prevents downloading private test bundles for
	// every runner invocation.
	PrivateBundlesStampPath string
	// OSVersion contains the value of CHROMEOS_RELEASE_BUILDER_PATH in /etc/lsb-release
	// or combination of CHROMEOS_RELEASE_BOARD, CHROMEOS_RELEASE_CHROME_MILESTONE,
	// CHROMEOS_RELEASE_BUILD_TYPE and CHROMEOS_RELEASE_VERSION if CHROMEOS_RELEASE_BUILDER_PATH
	// is not available in /etc/lsb-release
	OSVersion string
}

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further populated by parsing clArgs or
// (if clArgs is empty, as is the case when a runner is executed by the tast command) by
// decoding a JSON-marshaled Args struct from stdin.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer, args *Args, cfg *Config) error {
	if len(clArgs) == 0 {
		if err := json.NewDecoder(stdin).Decode(args); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
		}
		args.report = true
	} else {
		// Expose a limited amount of configurability via command-line flags to support running test runners manually.
		args.Mode = RunTestsMode
		if args.RunTests == nil {
			args.RunTests = &RunTestsArgs{}
		}
		var extraUSEFlags []string

		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(stderr)
		const usage = `Usage: %s [flag]... [pattern]...

Run Tast tests matched by zero or more patterns.

This executes test bundles to run Tast tests and is typically executed by the
"tast" command. It can be executed manually to e.g. perform stress testing.

Exits with 0 if all tests passed and with a non-zero exit code for all other
errors, including the failure of an individual test.

`
		flags.Usage = func() {
			fmt.Fprintf(stderr, usage, filepath.Base(os.Args[0]))
			flags.PrintDefaults()
		}
		flags.StringVar(&args.RunTests.BundleGlob, "bundles",
			args.RunTests.BundleGlob, "glob matching test bundles")
		flags.StringVar(&args.RunTests.BundleArgs.DataDir, "datadir",
			args.RunTests.BundleArgs.DataDir, "directory containing data files")
		flags.StringVar(&args.RunTests.BundleArgs.OutDir, "outdir",
			args.RunTests.BundleArgs.OutDir, "base directory to write output files to")
		flags.Var(command.NewListFlag(",", func(v []string) { args.RunTests.Devservers = v }, nil),
			"devservers", "comma-separated list of devserver URLs")
		flags.Var(command.NewListFlag(",", func(v []string) { extraUSEFlags = v }, nil),
			"extrauseflags", "comma-separated list of additional USE flags to inject when checking test dependencies")
		flags.BoolVar(&args.RunTests.BundleArgs.WaitUntilReady, "waituntilready",
			true, "wait until DUT is ready before running tests")

		if cfg.Type == RemoteRunner {
			flags.StringVar(&args.RunTests.BundleArgs.Target, "target",
				"", "DUT connection spec as \"[<user>@]host[:<port>]\"")
			flags.StringVar(&args.RunTests.BundleArgs.KeyFile, "keyfile",
				"", "path to SSH private key to use for connecting to DUT")
			flags.StringVar(&args.RunTests.BundleArgs.KeyDir, "keydir",
				"", "directory containing SSH private keys (typically $HOME/.ssh)")
		}

		if err := flags.Parse(clArgs); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "%v", err)
		}
		args.RunTests.BundleArgs.Patterns = flags.Args()

		// When the runner is executed by the "tast run" command, the list of software features (used to skip
		// unsupported tests) is passed in after having been gathered by an earlier call to local_test_runner
		// with GetDUTInfoMode. When the runner is executed directly, gather the list here instead.
		if err := setManualDepsArgs(args, cfg, extraUSEFlags); err != nil {
			return err
		}
	}

	if (args.Mode == RunTestsMode && args.RunTests == nil) ||
		(args.Mode == ListTestsMode && args.ListTests == nil) ||
		(args.Mode == CollectSysInfoMode && args.CollectSysInfo == nil) ||
		(args.Mode == GetDUTInfoMode && args.GetDUTInfo == nil) ||
		(args.Mode == DownloadPrivateBundlesMode && args.DownloadPrivateBundles == nil) ||
		(args.Mode == ListFixturesMode && args.ListFixtures == nil) {
		return command.NewStatusErrorf(statusBadArgs, "args not set for mode %v", args.Mode)
	}

	args.fillDefaults(cfg)

	// Use deprecated fields if they were supplied by an old tast binary.
	args.PromoteDeprecated()

	return nil
}

// setManualDepsArgs sets dependency/feature-related fields in args.RunTests appropriately for a manual
// run (i.e. when the runner is executed directly with command-line flags rather than via "tast run").
func setManualDepsArgs(args *Args, cfg *Config, extraUSEFlags []string) error {
	if cfg.USEFlagsFile == "" {
		return nil
	}
	if _, err := os.Stat(cfg.USEFlagsFile); os.IsNotExist(err) {
		return nil
	}

	useFlags, err := readUSEFlagsFile(cfg.USEFlagsFile)
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	useFlags = append(useFlags, extraUSEFlags...)

	var autotestCaps map[string]autocaps.State
	if cfg.AutotestCapabilityDir != "" {
		// Ignore errors. autotest-capability is outside of Tast's control, and it's probably better to let
		// some unsupported video tests fail instead of making the whole run fail.
		autotestCaps, _ = autocaps.Read(cfg.AutotestCapabilityDir, nil)
	}

	features, err := determineSoftwareFeatures(cfg.SoftwareFeatureDefinitions, useFlags, autotestCaps)
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	args.RunTests.BundleArgs.CheckSoftwareDeps = true
	args.RunTests.BundleArgs.AvailableSoftwareFeatures = features.Available
	args.RunTests.BundleArgs.UnavailableSoftwareFeatures = features.Unavailable
	return nil
}
