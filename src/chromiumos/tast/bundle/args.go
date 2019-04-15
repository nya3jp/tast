// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"chromiumos/tast/command"
	"chromiumos/tast/testing"
)

// RunMode describes the bundle's behavior.
type RunMode int

const (
	// RunTestsMode indicates that the bundle should run all matched tests and write the results to stdout as
	// a sequence of JSON-marshaled control.Test* control messages.
	RunTestsMode RunMode = 0
	// ListTestsMode indicates that the bundle should write information about matched tests to stdout as a
	// JSON array of testing.Test structs and exit.
	ListTestsMode = 1
)

// Args is used to pass arguments from test runners to test bundles.
// The runner executable writes the struct's JSON-marshaled representation to the bundle's stdin.
type Args struct {
	// Mode describes the mode that should be used by the bundle.
	Mode RunMode `json:"mode"`

	// RunTests contains arguments used by RunTestsMode.
	RunTests *RunTestsArgs `json:"runTests,omitempty"`
	// ListTests contains arguments used by ListTestsMode.
	ListTests *ListTestsArgs `json:"listTests,omitempty"`

	// TODO(derat): Delete these fields after 20190601: https://crbug.com/932307
	// PatternsDeprecated has been replaced by RunTests.Patterns and ListTests.Patterns.
	PatternsDeprecated []string `json:"patterns,omitempty"`
	// DataDirDeprecated has been replaced by RunTests.DataDir.
	DataDirDeprecated string `json:"dataDir,omitempty"`
	// OutDirDeprecated has been replaced by RunTests.OutDir.
	OutDirDeprecated string `json:"outDir,omitempty"`
	// TempDirDeprecated has been replaced by RunTests.TempDir.
	TempDirDeprecated string `json:"tempDir,omitempty"`
	// TargetDeprecated has been replaced by RunTests.Target.
	TargetDeprecated string `json:"remoteTarget,omitempty"`
	// KeyFileDeprecated has been replaced by RunTests.KeyFile.
	KeyFileDeprecated string `json:"remoteKeyFile,omitempty"`
	// KeyDirDeprecated has been replaced by RunTests.KeyDir.
	KeyDirDeprecated string `json:"remoteKeyDir,omitempty"`
	// TastPathDeprecated has been replaced by RunTests.TastPath.
	TastPathDeprecated string `json:"remoteTastPath,omitempty"`
	// RunFlagsDeprecated has been replaced by RunTests.RunFlags.
	RunFlagsDeprecated []string `json:"remoteRunArgs,omitempty"`
	// CheckSoftwareDepsDeprecated has been replaced by RunTests.CheckSoftwareDeps.
	CheckSoftwareDepsDeprecated bool `json:"runTestsCheckSoftwareDeps,omitempty"`
	// AvailableSoftwareFeaturesDeprecated has been replaced by RunTests.AvailableSoftwareFeatures.
	AvailableSoftwareFeaturesDeprecated []string `json:"runTestsAvailableSoftwareFeatures,omitempty"`
	// UnavailableSoftwareFeaturesDeprecated has been replaced by RunTests.UnavailableSoftwareFeatures.
	UnavailableSoftwareFeaturesDeprecated []string `json:"runTestsUnavailableSoftwareFeatures,omitempty"`
	// WaitUntilReadyDeprecated has been replaced by RunTests.WaitUntilReady.
	WaitUntilReadyDeprecated bool `json:"runTestsWaitUntilReady,omitempty"`
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by test runners to ensure that args will be interpreted
// correctly by older test bundles.
func (a *Args) FillDeprecated() {
	switch a.Mode {
	case RunTestsMode:
		if a.RunTests != nil {
			command.CopyFieldIfNonZero(&a.RunTests.Patterns, &a.PatternsDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.DataDir, &a.DataDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.OutDir, &a.OutDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.TempDir, &a.TempDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.Target, &a.TargetDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.KeyFile, &a.KeyFileDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.KeyDir, &a.KeyDirDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.TastPath, &a.TastPathDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.RunFlags, &a.RunFlagsDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.CheckSoftwareDeps, &a.CheckSoftwareDepsDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.AvailableSoftwareFeatures, &a.AvailableSoftwareFeaturesDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.UnavailableSoftwareFeatures, &a.UnavailableSoftwareFeaturesDeprecated)
			command.CopyFieldIfNonZero(&a.RunTests.WaitUntilReady, &a.WaitUntilReadyDeprecated)
		}
	case ListTestsMode:
		if a.ListTests != nil {
			command.CopyFieldIfNonZero(&a.ListTests.Patterns, &a.PatternsDeprecated)
		}
	}
}

// PromoteDeprecated copies all non-zero-valued deprecated fields to the corresponding non-deprecated fields.
// Missing sub-structs (e.g. RunTestsArgs and ListTestsArgs) are initialized.
// This method is called by test bundles to normalize args that were marshaled by an older test runner.
//
// If both an old and new field are set, the old field takes precedence. This is counter-intuitive but
// necessary: a default value for the new field may have been passed to run by Local or Remote. If the
// corresponding old field is non-zero, it was passed by an old runner (or by a new runner that called
// FillDeprecated), so we use the old field to make sure that it overrides the default.
func (a *Args) PromoteDeprecated() {
	switch a.Mode {
	case RunTestsMode:
		if a.RunTests == nil {
			a.RunTests = &RunTestsArgs{}
		}
		command.CopyFieldIfNonZero(&a.PatternsDeprecated, &a.RunTests.Patterns)
		command.CopyFieldIfNonZero(&a.DataDirDeprecated, &a.RunTests.DataDir)
		command.CopyFieldIfNonZero(&a.OutDirDeprecated, &a.RunTests.OutDir)
		command.CopyFieldIfNonZero(&a.TempDirDeprecated, &a.RunTests.TempDir)
		command.CopyFieldIfNonZero(&a.TargetDeprecated, &a.RunTests.Target)
		command.CopyFieldIfNonZero(&a.KeyFileDeprecated, &a.RunTests.KeyFile)
		command.CopyFieldIfNonZero(&a.KeyDirDeprecated, &a.RunTests.KeyDir)
		command.CopyFieldIfNonZero(&a.TastPathDeprecated, &a.RunTests.TastPath)
		command.CopyFieldIfNonZero(&a.RunFlagsDeprecated, &a.RunTests.RunFlags)
		command.CopyFieldIfNonZero(&a.CheckSoftwareDepsDeprecated, &a.RunTests.CheckSoftwareDeps)
		command.CopyFieldIfNonZero(&a.AvailableSoftwareFeaturesDeprecated, &a.RunTests.AvailableSoftwareFeatures)
		command.CopyFieldIfNonZero(&a.UnavailableSoftwareFeaturesDeprecated, &a.RunTests.UnavailableSoftwareFeatures)
		command.CopyFieldIfNonZero(&a.WaitUntilReadyDeprecated, &a.RunTests.WaitUntilReady)
	case ListTestsMode:
		if a.ListTests == nil {
			a.ListTests = &ListTestsArgs{}
		}
		command.CopyFieldIfNonZero(&a.PatternsDeprecated, &a.ListTests.Patterns)
	}
}

// RunTestsArgs is nested within Args and contains arguments used by RunTestsMode.
type RunTestsArgs struct {
	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns,omitempty"`

	// DataDir is the path to the directory containing test data files.
	DataDir string `json:"dataDir,omitempty"`
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string `json:"outDir,omitempty"`
	// TempDir is the path to the directory under which temporary files for tests are written.
	TempDir string `json:"tempDir,omitempty"`

	// Target is the DUT connection spec as [<user>@]host[:<port>].
	// It is only relevant for remote tests.
	Target string `json:"target,omitempty"`
	// KeyFile is the path to the SSH private key to use to connect to the DUT.
	// It is only relevant for remote tests.
	KeyFile string `json:"keyFile,omitempty"`
	// KeyDir is the directory containing SSH private keys (typically $HOME/.ssh).
	// It is only relevant for remote tests.
	KeyDir string `json:"keyDir,omitempty"`
	// TastPath contains the path to the tast binary that was executed to initiate testing.
	// It is only relevant for remote tests.
	TastPath string `json:"tastPath,omitempty"`
	// RunFlags contains a subset of the flags that were passed to the "tast run" command.
	// The included flags are ones that are necessary for core functionality,
	// e.g. paths to binaries used by the tast process and credentials for reconnecting to the DUT.
	// It is only relevant for remote tests.
	RunFlags []string `json:"runFlags,omitempty"`

	// CheckSoftwareDeps is true if each test's SoftwareDeps field should be checked against
	// AvailableSoftwareFeatures and UnavailableSoftwareFeatures.
	CheckSoftwareDeps bool `json:"checkSoftwareDeps,omitempty"`
	// AvailableSoftwareFeatures contains a list of software features supported by the DUT.
	AvailableSoftwareFeatures []string `json:"availableSoftwareFeatures,omitempty"`
	// UnavailableSoftwareFeatures contains a list of software features supported by the DUT.
	UnavailableSoftwareFeatures []string `json:"unavailableSoftwareFeatures,omitempty"`

	// WaitUntilReady indicates that the test bundle's "ready" function (see ReadyFunc) should
	// be executed before any tests are executed.
	WaitUntilReady bool `json:"waitUntilReady,omitempty"`
}

// ListTestsArgs is nested within Args and contains arguments used by ListTestsMode.
type ListTestsArgs struct {
	// Patterns contains patterns (either empty to list all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to list.
	Patterns []string `json:"patterns,omitempty"`
}

// bundleType describes the type of tests contained in a test bundle (i.e. local or remote).
type bundleType int

const (
	localBundle bundleType = iota
	remoteBundle
)

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further updated by decoding a JSON-marshaled Args struct from stdin.
// Matched tests are returned. The caller is responsible for performing the requested action.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer,
	args *Args, cfg *runConfig, bt bundleType) ([]*testing.Test, error) {
	if len(clArgs) != 0 {
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(stderr)
		flags.Usage = func() {
			runner := "local_test_runner"
			if bt == remoteBundle {
				runner = "remote_test_runner"
			}
			fmt.Fprintf(stderr, "Usage: %s [flag]...\n\n"+
				"Tast test bundle containing integration tests.\n\n"+
				"This is typically executed by %s.\n\n",
				filepath.Base(os.Args[0]), runner)
			flags.PrintDefaults()
		}

		dump := flags.Bool("dumptests", false, "dump all tests as a JSON-marshaled array of testing.Test structs")
		if err := flags.Parse(clArgs); err != nil {
			return nil, command.NewStatusErrorf(statusBadArgs, "%v", err)
		}
		if *dump {
			args.Mode = ListTestsMode
			return testing.GlobalRegistry().AllTests(), nil
		}
	}

	if err := json.NewDecoder(stdin).Decode(args); err != nil {
		return nil, command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
	}

	// Use non-zero-valued deprecated fields if they were supplied by an old test runner.
	args.PromoteDeprecated()

	if errs := testing.RegistrationErrors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		return nil, command.NewStatusErrorf(statusBadTests, "error(s) in registered tests: %v", strings.Join(es, ", "))
	}

	var patterns []string
	switch args.Mode {
	case RunTestsMode:
		patterns = args.RunTests.Patterns
	case ListTestsMode:
		patterns = args.ListTests.Patterns
	default:
		return nil, command.NewStatusErrorf(statusBadArgs, "invalid mode %d", args.Mode)
	}

	tests, err := testsToRun(patterns)
	if err != nil {
		return nil, command.NewStatusErrorf(statusBadPatterns, "failed getting tests for %v: %v", patterns, err.Error())
	}
	for _, tp := range tests {
		if tp.Timeout == 0 {
			tp.Timeout = cfg.defaultTestTimeout
		}
	}
	testing.SortTests(tests)
	return tests, nil
}

// TestPatternType describes the manner in which test patterns will be interpreted.
type TestPatternType int

const (
	// TestPatternGlobs means the patterns will be interpreted as one or more globs (possibly literal test names).
	TestPatternGlobs TestPatternType = iota
	// TestPatternAttrExpr means the patterns will be interpreted as a boolean expression referring to test attributes.
	TestPatternAttrExpr
)

// GetTestPatternType returns the manner in which test patterns pats will be interpreted.
// This is exported so it can be used by the tast command.
func GetTestPatternType(pats []string) TestPatternType {
	switch {
	case len(pats) == 1 && strings.HasPrefix(pats[0], "(") && strings.HasSuffix(pats[0], ")"):
		return TestPatternAttrExpr
	default:
		return TestPatternGlobs
	}
}

// testsToRun returns tests to run for a command invoked with test patterns pats.
// If no patterns are supplied, all registered tests are returned.
// If a single pattern is supplied and it is surrounded by parentheses,
// it is treated as a boolean expression specifying test attributes.
// Otherwise, pattern(s) are interpreted as globs matching test names.
func testsToRun(pats []string) ([]*testing.Test, error) {
	switch GetTestPatternType(pats) {
	case TestPatternGlobs:
		if len(pats) == 0 {
			return testing.GlobalRegistry().AllTests(), nil
		}
		// Print a helpful error message if it looks like the user wanted an attribute expression.
		if len(pats) == 1 && (strings.Contains(pats[0], "&&") || strings.Contains(pats[0], "||")) {
			return nil, fmt.Errorf("attr expr %q must be within parentheses", pats[0])
		}
		return testing.GlobalRegistry().TestsForGlobs(pats)
	case TestPatternAttrExpr:
		return testing.GlobalRegistry().TestsForAttrExpr(pats[0][1 : len(pats[0])-1])
	}
	return nil, fmt.Errorf("invalid test pattern(s) %v", pats)
}
