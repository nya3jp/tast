// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
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

// Args provides a backward- and forward-compatible way to pass arguments from the tast executable to test runners.
// The tast executable writes the struct's JSON-serialized representation to the runner's stdin.
type Args struct {
	// Mode describes the mode that should be used by the runner.
	Mode RunMode `json:"mode"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns,omitempty"`
	// DataDir is the path to the directory containing test data files.
	DataDir string `json:"dataDir,omitempty"`
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string `json:"outDir,omitempty"`
	// TempDir is the path to the directory under which temporary files for tests are written.
	TempDir string `json:"tempDir,omitempty"`

	// RemoteArgs contains additional arguments used to run remote tests.
	RemoteArgs
	// RunTestsArgs contains additional arguments used by RunTestsMode.
	RunTestsArgs
	// CollectSysInfoArgs contains additional arguments used by CollectSysInfoMode.
	CollectSysInfoArgs
	// GetSoftwareFeaturesArgs contains additional arguments used by GetSoftwareFeaturesMode.
	GetSoftwareFeaturesArgs

	// The remaining exported fields are set by test runner main functions (or by unit tests) and
	// cannot be overridden by the tast executable.

	// SystemLogDir contains the directory where information is logged by syslog and other daemons.
	SystemLogDir string `json:"-"`
	// SystemCrashDirs contains directories where crash dumps are written when processes crash.
	SystemCrashDirs []string `json:"-"`

	// USEFlagsFile contains the path to a file listing a subset of USE flags that were set when building
	// the system image. These USE flags are used by expressions in SoftwareFeatureDefinitions to determine
	// available software features.
	USEFlagsFile string `json:"-"`
	// SoftwareFeatureDefinitions maps from software feature names (e.g. "myfeature") to boolean expressions
	// used to compose them from USE flags (e.g. "a && !(b || c)"). The USE flags used in these expressions
	// must be listed in USEFlagsFile if they were set when building the system image.
	// See chromiumos/tast/expr for details about expression syntax.
	SoftwareFeatureDefinitions map[string]string `json:"-"`
	// AutotestCapabilityDir contains the path to a directory containing autotest-capability YAML files used to
	// define the DUT's capabilities for the purpose of determining which video tests it is able to run.
	// See https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/autotest-capability-default/
	// and the autocaps package for more information.
	AutotestCapabilityDir string `json:"-"`

	// report is set to true by readArgs if status should be reported via control messages rather
	// than human-readable log messages. This is true when args were supplied via stdin rather than
	// command-line flags, indicating that the runner was executed by the tast command. It's only relevant
	// for RunTestsMode.
	report bool

	// bundleArgs is filled by readArgs with arguments that should be passed to test bundles.
	bundleArgs bundle.Args
}

// RemoteArgs is nested within Args and holds additional arguments that are only relevant when running remote tests.
type RemoteArgs struct {
	bundle.RemoteArgs
	// Runner-specific args can be added here.
}

// RunTestsArgs is nested within Args and contains additional arguments used by RunTestsMode.
type RunTestsArgs struct {
	bundle.RunTestsArgs
	// Runner-specific args can be added here.
}

// GetSysInfoStateResult holds the result of a GetSysInfoStateMode command.
type GetSysInfoStateResult struct {
	// SysInfoState contains the collected state.
	State SysInfoState `json:"state"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
	// Each warning can be logged directly without additional information.
	Warnings []string `json:"warnings"`
}

// CollectSysInfoArgs is nested within Args and holds additional arguments that are only relevant for CollectSysInfoMode.
type CollectSysInfoArgs struct {
	// InitialState describes the pre-testing state of the DUT. It should be generated by a GetSysInfoStateMode
	// command executed before tests are run.
	InitialState SysInfoState `json:"collectSysInfoInitialState,omitempty"`
}

// CollectSysInfoResult contains the result of a CollectSysInfoMode command.
type CollectSysInfoResult struct {
	// LogDir is the directory where log files were copied. The caller should delete it.
	LogDir string `json:"logDir"`
	// CrashDir is the directory where minidump crash files were copied. The caller should delete it.
	CrashDir string `json:"crashDir"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
	Warnings []string `json:"warnings"`
}

// GetSoftwareFeaturesArgs is nested within Args and contains additional arguments used by GetSoftwareFeaturesMode.
type GetSoftwareFeaturesArgs struct {
	// ExtraUSEFlags lists additional USE flags that should be treated as being set an addition to
	// the ones read from Args.USEFlagsFile when computing the feature sets for GetSoftwareFeaturesResult.
	ExtraUSEFlags []string `json:"getSoftwareFeaturesExtraUseFlags,omitempty"`
}

// GetSoftwareFeaturesResult contains the result of a GetSoftwareFeaturesMode command.
type GetSoftwareFeaturesResult struct {
	// Available contains a list of software features supported by the DUT.
	Available []string `json:"available"`
	// Unavailable contains a list of software features not supported by the DUT.
	Unavailable []string `json:"missing"`
	// Warnings contains descriptions of non-fatal errors encountered while determining features.
	Warnings []string `json:"warnings"`
}

// SysInfoState contains the state of the DUT's system information.
type SysInfoState struct {
	// LogInodeSizes maps from each log file's inode to its size in bytes.
	LogInodeSizes map[uint64]int64 `json:"logInodeSizes"`
	// MinidumpPaths contains absolute paths to minidump crash files.
	MinidumpPaths []string `json:"minidumpPaths"`
}

// RunnerType describes the type of test runner that is using this package.
type RunnerType int // NOLINT

const (
	// LocalRunner indicates that this package is being used by local_test_runner.
	LocalRunner RunnerType = iota
	// RemoteRunner indicates that this package is being used by remote_test_runner.
	RemoteRunner
)

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further populated by parsing clArgs or
// (if clArgs is empty, as is the case when a runner is executed by the tast command) by
// decoding a JSON-marshaled Args struct from stdin.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer, args *Args, runnerType RunnerType) error {
	if len(clArgs) == 0 {
		if err := json.NewDecoder(stdin).Decode(args); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
		}
		args.report = true
	} else {
		// Expose a limited amount of configurability via command-line flags to support running test runners manually.
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(stderr)
		flags.Usage = func() {
			fmt.Fprintf(stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
				"Runs tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
			flags.PrintDefaults()
		}
		flags.StringVar(&args.BundleGlob, "bundles", args.BundleGlob, "glob matching test bundles")
		flags.StringVar(&args.DataDir, "datadir", args.DataDir, "directory containing data files")
		flags.StringVar(&args.OutDir, "outdir", args.OutDir, "base directory to write output files to")
		flags.Var(command.NewListFlag(",", func(v []string) { args.GetSoftwareFeaturesArgs.ExtraUSEFlags = v }, nil),
			"extrauseflags", "comma-separated list of additional USE flags to inject when checking test dependencies")

		if runnerType == RemoteRunner {
			flags.StringVar(&args.Target, "target", "", "DUT connection spec as \"[<user>@]host[:<port>]\"")
			flags.StringVar(&args.KeyFile, "keyfile", "", "path to SSH private key to use for connecting to DUT")
			flags.StringVar(&args.KeyDir, "keydir", "", "directory containing SSH private keys (typically $HOME/.ssh)")
		}

		if err := flags.Parse(clArgs); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "%v", err)
		}
		args.Mode = RunTestsMode
		args.Patterns = flags.Args()

		// When the runner is executed by the "tast run" command, the list of software features (used to skip
		// unsupported tests) is passed in after having been gathered by an earlier call to local_test_runner
		// with GetSoftwareFeaturesMode. When the runner is executed directly, gather the list here instead.
		if err := setManualDepsArgs(args); err != nil {
			return err
		}
	}

	// Copy over args that need to be passed to test bundles.
	args.bundleArgs = bundle.Args{
		DataDir:      args.DataDir,
		OutDir:       args.OutDir,
		TempDir:      args.TempDir,
		Patterns:     args.Patterns,
		RunTestsArgs: args.RunTestsArgs.RunTestsArgs,
	}
	if !reflect.DeepEqual(args.RemoteArgs, RemoteArgs{}) {
		if runnerType != RemoteRunner {
			return command.NewStatusErrorf(statusBadArgs, "remote args %+v passed to non-remote runner", args.RemoteArgs)
		}
		args.bundleArgs.RemoteArgs = args.RemoteArgs.RemoteArgs
	}
	return nil
}

// setManualDepsArgs sets dependency/feature-related fields in args.RunTestArgs appropriately for a manual
// run (i.e. when the runner is executed directly with command-line flags rather than via "tast run").
func setManualDepsArgs(args *Args) error {
	if bundle.GetTestPatternType(args.Patterns) != bundle.TestPatternAttrExpr || args.USEFlagsFile == "" {
		return nil
	}
	if _, err := os.Stat(args.USEFlagsFile); os.IsNotExist(err) {
		return nil
	}

	useFlags, err := readUSEFlagsFile(args.USEFlagsFile)
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	useFlags = append(useFlags, args.ExtraUSEFlags...)

	var autotestCaps map[string]autocaps.State
	if args.AutotestCapabilityDir != "" {
		// Ignore errors. autotest-capability is outside of Tast's control, and it's probably better to let
		// some unsupported video tests fail instead of making the whole run fail.
		autotestCaps, _ = autocaps.Read(args.AutotestCapabilityDir, nil)
	}

	avail, unavail, err := determineSoftwareFeatures(args.SoftwareFeatureDefinitions, useFlags, autotestCaps)
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	args.RunTestsArgs.CheckSoftwareDeps = true
	args.RunTestsArgs.AvailableSoftwareFeatures = avail
	args.RunTestsArgs.UnavailableSoftwareFeatures = unavail
	return nil
}

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
)
