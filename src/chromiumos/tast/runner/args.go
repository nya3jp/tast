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
	"reflect"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/bundle"
	"chromiumos/tast/command"
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
	// GetSoftwareFeaturesMode indicates that the runner should return information about software features
	// supported by the DUT via a JSON-marshaled GetSoftwareFeaturesResult struct written to stdout. This mode
	// is only supported by local_test_runner.
	GetSoftwareFeaturesMode = 5
	// DownloadPrivateBundlesMode indicates that the runner should download private bundles from devservers,
	// install them to the DUT, write a JSON-marshaled DownloadPrivateBundlesResult struct to stdout and exit.
	// This mode is only supported by local_test_runner.
	DownloadPrivateBundlesMode = 6
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
	// CollectSysInfo contains arguments used by CollectSysInfoMode.
	CollectSysInfo *CollectSysInfoArgs `json:"collectSysInfo,omitempty"`
	// GetSoftwareFeatures contains arguments used by GetSoftwareFeaturesMode.
	GetSoftwareFeatures *GetSoftwareFeaturesArgs `json:"getSoftwareFeatures,omitempty"`
	// DownloadPrivateBundles contains arguments used by DownloadPrivateBundlesMode.
	DownloadPrivateBundles *DownloadPrivateBundlesArgs `json:"downloadPrivateBundles,omitempty"`

	// report is set to true by readArgs if status should be reported via control messages rather
	// than human-readable log messages. This is true when args were supplied via stdin rather than
	// command-line flags, indicating that the runner was executed by the tast command. It's only relevant
	// for RunTestsMode.
	report bool

	// TODO(derat): Delete these fields after 20190601: https://crbug.com/932307
	// BundleGlob has been replaced by RunTests.BundleGlob and ListTests.BundleGlob.
	BundleGlobDeprecated string `json:"bundleGlob,omitempty"`
	// PatternsDeprecated has been replaced by RunTests.BundleArgs.Patterns and ListTests.BundleArgs.Patterns.
	PatternsDeprecated []string `json:"patterns,omitempty"`
	// TargetDeprecated has been replaced by RunTests.BundleArgs.Target.
	TargetDeprecated string `json:"remoteTarget,omitempty"`
	// KeyFileDeprecated has been replaced by RunTests.BundleArgs.KeyFile.
	KeyFileDeprecated string `json:"remoteKeyFile,omitempty"`
	// KeyDirDeprecated has been replaced by RunTests.BundleArgs.KeyDir.
	KeyDirDeprecated string `json:"remoteKeyDir,omitempty"`
	// TastPathDeprecated has been replaced by RunTests.BundleArgs.TastPath.
	TastPathDeprecated string `json:"remoteTastPath,omitempty"`
	// RunFlagsDeprecated has been replaced by RunTests.BundleArgs.RunFlags.
	RunFlagsDeprecated []string `json:"remoteRunArgs,omitempty"`
	// DataDirDeprecated has been replaced by RunTests.BundleArgs.DataDir.
	DataDirDeprecated string `json:"dataDir,omitempty"`
	// OutDirDeprecated has been replaced by RunTests.BundleArgs.OutDir.
	OutDirDeprecated string `json:"outDir,omitempty"`
	// TempDirDeprecated has been replaced by RunTests.BundleArgs.TempDir.
	TempDirDeprecated string `json:"tempDir,omitempty"`
	// CheckSoftwareDepsDeprecated has been replaced by RunTests.BundleArgs.CheckSoftwareDeps.
	CheckSoftwareDepsDeprecated bool `json:"runTestsCheckSoftwareDeps,omitempty"`
	// AvailableSoftwareFeaturesDeprecated has been replaced by RunTests.BundleArgs.AvailableSoftwareFeatures.
	AvailableSoftwareFeaturesDeprecated []string `json:"runTestsAvailableSoftwareFeatures,omitempty"`
	// UnavailableSoftwareFeaturesDeprecated has been replaced by RunTests.BundleArgs.UnavailableSoftwareFeatures.
	UnavailableSoftwareFeaturesDeprecated []string `json:"runTestsUnavailableSoftwareFeatures,omitempty"`
	// WaitUntilReadyDeprecated has been replaced by RunTests.BundleArgs.WaitUntilReady.
	WaitUntilReadyDeprecated bool `json:"runTestsWaitUntilReady,omitempty"`
	// DevserversDeprecated has been replaced by RunTests.Devservers.
	DevserversDeprecated []string `json:"devservers,omitempty"`
	// InitialStateDeprecated has been replaced by CollectSysInfo.InitialState.
	InitialStateDeprecated SysInfoState `json:"collectSysInfoInitialState,omitempty"`
	// ExtraUSEFlagsDeprecated has been replaced by GetSoftwareFeatures.ExtraUSEFlags.
	ExtraUSEFlagsDeprecated []string `json:"getSoftwareFeaturesExtraUseFlags,omitempty"`
	// DownloadPrivateBundelsDevserversDeprecated has been replaced by DownloadPrivateBundles.Devservers.
	DownloadPrivateBundlesDevserversDeprecated []string `json:"downloadPrivateBundlesDevservers,omitempty"`
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

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by the tast process to ensure that args will be interpreted
// correctly by older test runners.
func (a *Args) FillDeprecated() {
	switch a.Mode {
	case RunTestsMode:
		if a.RunTests != nil {
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.Patterns, &a.PatternsDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.Target, &a.TargetDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.KeyFile, &a.KeyFileDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.KeyDir, &a.KeyDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.TastPath, &a.TastPathDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.RunFlags, &a.RunFlagsDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.DataDir, &a.DataDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.OutDir, &a.OutDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.TempDir, &a.TempDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.CheckSoftwareDeps, &a.CheckSoftwareDepsDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.AvailableSoftwareFeatures, &a.AvailableSoftwareFeaturesDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.UnavailableSoftwareFeatures, &a.UnavailableSoftwareFeaturesDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.WaitUntilReady, &a.WaitUntilReadyDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.BundleGlob, &a.BundleGlobDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.Devservers, &a.DevserversDeprecated)
		}
	case ListTestsMode:
		if a.ListTests != nil {
			command.CopyFieldIfNonZero(&a.ListTests.BundleArgs.Patterns, &a.PatternsDeprecated)
			command.CopyFieldIfNonZero(&a.ListTests.BundleGlob, &a.BundleGlobDeprecated)
		}
	case CollectSysInfoMode:
		if a.CollectSysInfo != nil {
			a.InitialStateDeprecated = a.CollectSysInfo.InitialState // copy struct manually
		}
	case GetSoftwareFeaturesMode:
		if a.GetSoftwareFeatures != nil {
			command.CopyFieldIfNonZero(&a.GetSoftwareFeatures.ExtraUSEFlags, &a.ExtraUSEFlagsDeprecated)
		}
	case DownloadPrivateBundlesMode:
		if a.DownloadPrivateBundles != nil {
			command.CopyFieldIfNonZero(&a.DownloadPrivateBundles.Devservers, &a.DownloadPrivateBundlesDevserversDeprecated)
		}
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
	switch a.Mode {
	case RunTestsMode:
		if a.RunTests == nil {
			a.RunTests = &RunTestsArgs{}
		}
		command.CopyFieldIfNonZero(&a.PatternsDeprecated, &a.RunTests.BundleArgs.Patterns)
		command.CopyFieldIfNonZero(&a.TargetDeprecated, &a.RunTests.BundleArgs.Target)
		command.CopyFieldIfNonZero(&a.KeyFileDeprecated, &a.RunTests.BundleArgs.KeyFile)
		command.CopyFieldIfNonZero(&a.KeyDirDeprecated, &a.RunTests.BundleArgs.KeyDir)
		command.CopyFieldIfNonZero(&a.TastPathDeprecated, &a.RunTests.BundleArgs.TastPath)
		command.CopyFieldIfNonZero(&a.RunFlagsDeprecated, &a.RunTests.BundleArgs.RunFlags)
		command.CopyFieldIfNonZero(&a.DataDirDeprecated, &a.RunTests.BundleArgs.DataDir)
		command.CopyFieldIfNonZero(&a.OutDirDeprecated, &a.RunTests.BundleArgs.OutDir)
		command.CopyFieldIfNonZero(&a.TempDirDeprecated, &a.RunTests.BundleArgs.TempDir)
		command.CopyFieldIfNonZero(&a.CheckSoftwareDepsDeprecated, &a.RunTests.BundleArgs.CheckSoftwareDeps)
		command.CopyFieldIfNonZero(&a.AvailableSoftwareFeaturesDeprecated, &a.RunTests.BundleArgs.AvailableSoftwareFeatures)
		command.CopyFieldIfNonZero(&a.UnavailableSoftwareFeaturesDeprecated, &a.RunTests.BundleArgs.UnavailableSoftwareFeatures)
		command.CopyFieldIfNonZero(&a.WaitUntilReadyDeprecated, &a.RunTests.BundleArgs.WaitUntilReady)
		command.CopyFieldIfNonZero(&a.BundleGlobDeprecated, &a.RunTests.BundleGlob)
		command.CopyFieldIfNonZero(&a.DevserversDeprecated, &a.RunTests.Devservers)
	case ListTestsMode:
		if a.ListTests == nil {
			a.ListTests = &ListTestsArgs{}
		}
		command.CopyFieldIfNonZero(&a.PatternsDeprecated, &a.ListTests.BundleArgs.Patterns)
		command.CopyFieldIfNonZero(&a.BundleGlobDeprecated, &a.ListTests.BundleGlob)
	case CollectSysInfoMode:
		if a.CollectSysInfo == nil {
			a.CollectSysInfo = &CollectSysInfoArgs{}
		}
		if !reflect.DeepEqual(a.InitialStateDeprecated, SysInfoState{}) {
			a.CollectSysInfo.InitialState = a.InitialStateDeprecated
		}
	case GetSoftwareFeaturesMode:
		if a.GetSoftwareFeatures == nil {
			a.GetSoftwareFeatures = &GetSoftwareFeaturesArgs{}
		}
		command.CopyFieldIfNonZero(&a.ExtraUSEFlagsDeprecated, &a.GetSoftwareFeatures.ExtraUSEFlags)
	case DownloadPrivateBundlesMode:
		if a.DownloadPrivateBundles == nil {
			a.DownloadPrivateBundles = &DownloadPrivateBundlesArgs{}
		}
		command.CopyFieldIfNonZero(&a.DownloadPrivateBundlesDevserversDeprecated, &a.DownloadPrivateBundles.Devservers)
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
}

// ListTestsArgs is nested within Args and contains arguments used by ListTestsMode.
type ListTestsArgs struct {
	// BundleArgs contains arguments that are relevant to test bundles.
	BundleArgs bundle.ListTestsArgs `json:"bundleArgs"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
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

// GetSoftwareFeaturesArgs is nested within Args and contains arguments used by GetSoftwareFeaturesMode.
type GetSoftwareFeaturesArgs struct {
	// ExtraUSEFlags lists USE flags that should be treated as being set an addition to
	// the ones read from Config.USEFlagsFile when computing the feature sets for GetSoftwareFeaturesResult.
	ExtraUSEFlags []string `json:"extraUseFlags,omitempty"`
}

// GetSoftwareFeaturesResult contains the result of a GetSoftwareFeaturesMode command.
type GetSoftwareFeaturesResult struct {
	// Available contains a list of software features supported by the DUT.
	Available []string `json:"available,omitempty"`
	// Unavailable contains a list of software features not supported by the DUT.
	Unavailable []string `json:"missing,omitempty"`
	// Warnings contains descriptions of non-fatal errors encountered while determining features.
	Warnings []string `json:"warnings,omitempty"`
}

// SysInfoState contains the state of the DUT's system information.
type SysInfoState struct {
	// LogInodeSizes maps from each log file's inode to its size in bytes.
	LogInodeSizes map[uint64]int64 `json:"logInodeSizes,omitempty"`
	// JournaldCursor contains an opaque cursor pointing at the current tip of journald logs.
	JournaldCursor string `json:"journaldCursor,omitempty"`
	// MinidumpPaths contains absolute paths to minidump crash files.
	MinidumpPaths []string `json:"minidumpPaths,omitempty"`
}

// DownloadPrivateBundlesArgs is nested within Args and contains arguments used by DownloadPrivateBundlesMode.
type DownloadPrivateBundlesArgs struct {
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`
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

	// SystemLogDir contains the directory where information is logged by syslog and other daemons.
	SystemLogDir string
	// SystemLogExcludes contains relative paths of directories and files in SystemLogDir to exclude.
	SystemLogExcludes []string
	// JournaldSubdir contains the subdirectory within CollectSysInfoResult.LogDir where journald logs will be written.
	// No journald logs will be be collected if this is empty.
	JournaldSubdir string `json:"-"`
	// SystemInfoFunc contains a function that will be executed to gather additional system info.
	// The information should be written to dir.
	SystemInfoFunc func(ctx context.Context, dir string) error
	// SystemCrashDirs contains directories where crash dumps are written when processes crash.
	SystemCrashDirs []string

	// USEFlagsFile contains the path to a file listing a subset of USE flags that were set when building
	// the system image. These USE flags are used by expressions in SoftwareFeatureDefinitions to determine
	// available software features.
	USEFlagsFile string
	// SoftwareFeatureDefinitions maps from software feature names (e.g. "myfeature") to boolean expressions
	// used to compose them from USE flags (e.g. "a && !(b || c)"). The USE flags used in these expressions
	// must be listed in USEFlagsFile if they were set when building the system image.
	// See chromiumos/tast/expr for details about expression syntax.
	SoftwareFeatureDefinitions map[string]string
	// AutotestCapabilityDir contains the path to a directory containing autotest-capability YAML files used to
	// define the DUT's capabilities for the purpose of determining which video tests it is able to run.
	// See https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/autotest-capability-default/
	// and the autocaps package for more information.
	AutotestCapabilityDir string
	// BuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash, containing build
	// artifacts for the current Chrome OS image.
	BuildArtifactsURL string
	// PrivateBundleArchiveURL contains the URL of the private test bundles archive corresponding to the current
	// Chrome OS image.
	PrivateBundleArchiveURL string
	// PrivateBundlesStampPath contains the path to a stamp file indicating private test bundles have been
	// successfully downloaded and installed before. This prevents downloading private test bundles for
	// every runner invocation.
	PrivateBundlesStampPath string
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
		flags.Usage = func() {
			fmt.Fprintf(stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
				"Runs tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
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
			false, "wait until DUT is ready before running tests")

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
		// with GetSoftwareFeaturesMode. When the runner is executed directly, gather the list here instead.
		if err := setManualDepsArgs(args, cfg, extraUSEFlags); err != nil {
			return err
		}
	}

	// Use deprecated fields if they were supplied by an old tast binary.
	args.PromoteDeprecated()

	return nil
}

// setManualDepsArgs sets dependency/feature-related fields in args.RunTests appropriately for a manual
// run (i.e. when the runner is executed directly with command-line flags rather than via "tast run").
func setManualDepsArgs(args *Args, cfg *Config, extraUSEFlags []string) error {
	if bundle.GetTestPatternType(args.RunTests.BundleArgs.Patterns) != bundle.TestPatternAttrExpr {
		return nil
	}
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

	avail, unavail, err := determineSoftwareFeatures(cfg.SoftwareFeatureDefinitions, useFlags, autotestCaps)
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	args.RunTests.BundleArgs.CheckSoftwareDeps = true
	args.RunTests.BundleArgs.AvailableSoftwareFeatures = avail
	args.RunTests.BundleArgs.UnavailableSoftwareFeatures = unavail
	return nil
}
