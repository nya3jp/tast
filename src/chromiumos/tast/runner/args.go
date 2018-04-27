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

	// RemoteArgs contains additional arguments used to run remote tests.
	RemoteArgs
	// CollectSysInfoArgs contains additional arguments used by CollectSysInfoMode.
	CollectSysInfoArgs

	// SystemLogDir contains the directory where information is logged by syslog and other daemons.
	// It is set by the runner (or by unit tests) and cannot be overridden by the tast executable.
	SystemLogDir string `json:"-"`
	// SystemCrashDirs contains directories where crash dumps are written when processes crash.
	// It is set by the runner (or by unit tests) and cannot be overridden by the tast executable.
	SystemCrashDirs []string `json:"-"`

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
	// Target is the DUT connection spec as [<user>@]host[:<port>].
	Target string `json:"remoteTarget,omitempty"`
	// KeyFile is the path to the SSH private key to use to connect to the DUT.
	KeyFile string `json:"remoteKeyFile,omitempty"`
	// KeyDir is the directory containing SSH private keys (typically $HOME/.ssh).
	KeyDir string `json:"remoteKeyDir,omitempty"`
}

// GetSysInfoStateResult holds the result of a GetSysInfoStateMode command.
type GetSysInfoStateResult struct {
	// SysInfoState contains the collected state.
	State SysInfoState `json:"state"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
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

// SysInfoState contains the state of the DUT's system information.
type SysInfoState struct {
	// LogInodeSizes maps from each log file's inode to its size in bytes.
	LogInodeSizes map[uint64]int64 `json:"logInodeSizes"`
	// MinidumpPaths contains absolute paths to minidump crash files.
	MinidumpPaths []string `json:"minidumpPaths"`
}

// RunnerType describes the type of test runner that is using this package.
type RunnerType int

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
	}

	// Copy over args that need to be passed to test bundles.
	args.bundleArgs = bundle.Args{
		DataDir:  args.DataDir,
		OutDir:   args.OutDir,
		Patterns: args.Patterns,
	}
	if args.RemoteArgs != (RemoteArgs{}) {
		if runnerType != RemoteRunner {
			return command.NewStatusErrorf(statusBadArgs, "remote args %+v passed to non-remote runner", args.RemoteArgs)
		}
		args.bundleArgs.RemoteArgs = bundle.RemoteArgs{
			Target:  args.Target,
			KeyFile: args.KeyFile,
			KeyDir:  args.KeyDir,
		}
	}
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
)
