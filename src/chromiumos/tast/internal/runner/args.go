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

	"chromiumos/tast/autocaps"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
)

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
	// when using RunnerRunTestsMode. This can help prevent confusing failures if multiple test jobs are
	// incorrectly scheduled on the same DUT: https://crbug.com/941829
	KillStaleRunners bool

	// SystemLogDir contains the directory where information is logged by syslog and other daemons.
	SystemLogDir string
	// SystemLogExcludes contains relative paths of directories and files in SystemLogDir to exclude.
	SystemLogExcludes []string
	// UnifiedLogSubdir contains the subdirectory within RunnerCollectSysInfoResult.LogDir where unified system logs will be written.
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
	// See https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/HEAD/chromeos-base/autotest-capability-default/
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

// fillDefaults fills unset fields with default values from cfg.
func (cfg *Config) fillDefaults(a *jsonprotocol.RunnerArgs) {
	switch a.Mode {
	case jsonprotocol.RunnerRunTestsMode:
		if a.RunTests.BundleArgs.BuildArtifactsURL == "" {
			a.RunTests.BundleArgs.BuildArtifactsURL = cfg.DefaultBuildArtifactsURL
		}
	case jsonprotocol.RunnerDownloadPrivateBundlesMode:
		if a.DownloadPrivateBundles.BuildArtifactsURL == "" {
			a.DownloadPrivateBundles.BuildArtifactsURL = cfg.DefaultBuildArtifactsURL
		}
	}
}

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further populated by parsing clArgs or
// (if clArgs is empty, as is the case when a runner is executed by the tast command) by
// decoding a JSON-marshaled RunnerArgs struct from stdin.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer, args *jsonprotocol.RunnerArgs, cfg *Config) error {
	if len(clArgs) == 0 {
		if err := json.NewDecoder(stdin).Decode(args); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
		}
		args.Report = true
	} else {
		// Expose a limited amount of configurability via command-line flags to support running test runners manually.
		args.Mode = jsonprotocol.RunnerRunTestsMode
		if args.RunTests == nil {
			args.RunTests = &jsonprotocol.RunnerRunTestsArgs{}
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
		rpc := flags.Bool("rpc", false, "run gRPC server")
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

		if *rpc {
			args.Mode = jsonprotocol.RunnerRPCMode
			return nil
		}

		args.RunTests.BundleArgs.Patterns = flags.Args()

		// When the runner is executed by the "tast run" command, the list of software features (used to skip
		// unsupported tests) is passed in after having been gathered by an earlier call to local_test_runner
		// with RunnerGetDUTInfoMode. When the runner is executed directly, gather the list here instead.
		if err := setManualDepsArgs(args, cfg, extraUSEFlags); err != nil {
			return err
		}
	}

	if (args.Mode == jsonprotocol.RunnerRunTestsMode && args.RunTests == nil) ||
		(args.Mode == jsonprotocol.RunnerListTestsMode && args.ListTests == nil) ||
		(args.Mode == jsonprotocol.RunnerCollectSysInfoMode && args.CollectSysInfo == nil) ||
		(args.Mode == jsonprotocol.RunnerGetDUTInfoMode && args.GetDUTInfo == nil) ||
		(args.Mode == jsonprotocol.RunnerDownloadPrivateBundlesMode && args.DownloadPrivateBundles == nil) ||
		(args.Mode == jsonprotocol.RunnerListFixturesMode && args.ListFixtures == nil) {
		return command.NewStatusErrorf(statusBadArgs, "args not set for mode %v", args.Mode)
	}

	cfg.fillDefaults(args)

	// Use deprecated fields if they were supplied by an old tast binary.
	args.PromoteDeprecated()

	return nil
}

// setManualDepsArgs sets dependency/feature-related fields in args.RunTests appropriately for a manual
// run (i.e. when the runner is executed directly with command-line flags rather than via "tast run").
func setManualDepsArgs(args *jsonprotocol.RunnerArgs, cfg *Config, extraUSEFlags []string) error {
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
