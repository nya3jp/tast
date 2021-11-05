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

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
)

// RunnerType describes the type of test runner that is using this package.
type RunnerType int // NOLINT

const (
	// LocalRunner indicates that this package is being used by local_test_runner.
	LocalRunner RunnerType = iota
	// RemoteRunner indicates that this package is being used by remote_test_runner.
	RemoteRunner
)

// StaticConfig contains fixed parameters for the runner that are passed in from
// local_test_runner or remote_test_runner.
type StaticConfig struct {
	// Type describes the type of runner being executed.
	Type RunnerType

	// KillStaleRunners dictates whether SIGTERM should be sent to any existing test runner processes
	// when using RunnerRunTestsMode. This can help prevent confusing failures if multiple test jobs are
	// incorrectly scheduled on the same DUT: https://crbug.com/941829
	KillStaleRunners bool
	// EnableSyslog specifies whether to copy logs to syslog. It should be
	// always enabled on production, but can be disabled in unit tests to
	// avoid spamming syslog.
	EnableSyslog bool

	// GetDUTInfo is a function to respond to GetDUTInfo RPC.
	// If it is nil, an empty GetDUTInfoResponse is always returned.
	GetDUTInfo func(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)

	// GetSysInfoState is a function to respond to GetSysInfoState RPC.
	// If it is nil, an empty GetSysInfoStateResponse is always returned.
	GetSysInfoState func(ctx context.Context, req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error)

	// CollectSysInfo is a function to respond to CollectSysInfo RPC.
	// If it is nil, an empty CollectSysInfoResponse is always returned.
	CollectSysInfo func(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error)

	// DeprecatedDefaultBuildArtifactsURL is a function that computes the
	// default build artifacts URL.
	// If it is nil, an empty default build artifacts URL is used, which
	// will cause all build artifact downloads to fail.
	// DEPRECATED: Do not call this function except in fillDefaults.
	// Instead, Tast CLI should call GetDUTInfo RPC method first to know the
	// default URL and pass it in later requests.
	DeprecatedDefaultBuildArtifactsURL func() string

	// PrivateBundlesStampPath contains the path to a stamp file indicating private test bundles have been
	// successfully downloaded and installed before. This prevents downloading private test bundles for
	// every runner invocation.
	PrivateBundlesStampPath string
}

// fillDefaults fills unset fields with default values from cfg.
func (c *StaticConfig) fillDefaults(a *jsonprotocol.RunnerArgs) {
	switch a.Mode {
	case jsonprotocol.RunnerRunTestsMode:
		if a.RunTests.BundleArgs.BuildArtifactsURL == "" && c.DeprecatedDefaultBuildArtifactsURL != nil {
			a.RunTests.BundleArgs.BuildArtifactsURL = c.DeprecatedDefaultBuildArtifactsURL()
		}
	case jsonprotocol.RunnerDownloadPrivateBundlesMode:
		if a.DownloadPrivateBundles.BuildArtifactsURL == "" && c.DeprecatedDefaultBuildArtifactsURL != nil {
			a.DownloadPrivateBundles.BuildArtifactsURL = c.DeprecatedDefaultBuildArtifactsURL()
		}
	}
}

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further populated by parsing clArgs or
// (if clArgs is empty, as is the case when a runner is executed by the tast command) by
// decoding a JSON-marshaled RunnerArgs struct from stdin.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer, args *jsonprotocol.RunnerArgs, scfg *StaticConfig) error {
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

		if scfg.Type == RemoteRunner {
			flags.StringVar(&args.RunTests.BundleArgs.ConnectionSpec, "target",
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
		if scfg.GetDUTInfo != nil {
			req := &protocol.GetDUTInfoRequest{ExtraUseFlags: extraUSEFlags}
			res, err := scfg.GetDUTInfo(context.Background(), req)
			if err != nil {
				return err
			}

			fs := res.GetDutInfo().GetFeatures().GetSoftware()
			args.RunTests.BundleArgs.CheckDeps = true
			args.RunTests.BundleArgs.AvailableSoftwareFeatures = fs.GetAvailable()
			args.RunTests.BundleArgs.UnavailableSoftwareFeatures = fs.GetUnavailable()
			// Historically we set software features only. Do not bother to
			// improve hardware feature support in direct mode.
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

	scfg.fillDefaults(args)

	// Use deprecated fields if they were supplied by an old tast binary.
	args.PromoteDeprecated()

	return nil
}
