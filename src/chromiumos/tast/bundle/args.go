// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
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
	// Patterns contains patterns (either empty to run/list all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run/list.
	Patterns []string `json:"patterns,omitempty"`

	// Remote contains additional arguments used to run remote tests.
	Remote RemoteArgs `json:"remote,omitempty"`
	// RunTests contains additional arguments used by RunTestsMode.
	RunTests RunTestsArgs `json:"runTests,omitempty"`

	// TODO(derat): Delete these fields after 20190501: https://crbug.com/932307
	// TargetDeprecated has been replaced by Remote.Target.
	TargetDeprecated string `json:"remoteTarget,omitempty"`
	// KeyFileDeprecated has been replaced by Remote.KeyFile.
	KeyFileDeprecated string `json:"remoteKeyFile,omitempty"`
	// KeyDirDeprecated has been replaced by Remote.KeyDir.
	KeyDirDeprecated string `json:"remoteKeyDir,omitempty"`
	// TastPathDeprecated has been replaced by Remote.TastPath.
	TastPathDeprecated string `json:"remoteTastPath,omitempty"`
	// RunFlagsDeprecated has been replaced by Remote.RunFlags.
	RunFlagsDeprecated []string `json:"remoteRunArgs,omitempty"`
	// DataDirDeprecated has been replaced by RunTests.DataDir.
	DataDirDeprecated string `json:"dataDir,omitempty"`
	// OutDirDeprecated has been replaced by RunTests.OutDir.
	OutDirDeprecated string `json:"outDir,omitempty"`
	// TempDirDeprecated has been replaced by RunTests.TempDir.
	TempDirDeprecated string `json:"tempDir,omitempty"`
	// CheckSoftwareDepsDeprecated has been replaced by RunTests.CheckSoftwareDeps.
	CheckSoftwareDepsDeprecated bool `json:"runTestsCheckSoftwareDeps,omitempty"`
	// AvailableSoftwareFeaturesDeprecated has been replaced by RunTests.AvailableSoftwareFeatures.
	AvailableSoftwareFeaturesDeprecated []string `json:"runTestsAvailableSoftwareFeatures,omitempty"`
	// UnavailableSoftwareFeaturesDeprecated has been replaced by RunTests.UnavailableSoftwareFeatures.
	UnavailableSoftwareFeaturesDeprecated []string `json:"runTestsUnavailableSoftwareFeatures,omitempty"`
	// WaitUntilReadyDeprecated has been replaced by RunTests.WaitUntilReady.
	WaitUntilReadyDeprecated bool `json:"runTestsWaitUntilReady,omitempty"`
}

// deprecatedFields returns a mapping from pointers to deprecated fields in Args to
// pointers to the corresponding non-deprecated fields.
func (a *Args) deprecatedFields() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		&a.TargetDeprecated:                      &a.Remote.Target,
		&a.KeyFileDeprecated:                     &a.Remote.KeyFile,
		&a.KeyDirDeprecated:                      &a.Remote.KeyDir,
		&a.TastPathDeprecated:                    &a.Remote.TastPath,
		&a.RunFlagsDeprecated:                    &a.Remote.RunFlags,
		&a.DataDirDeprecated:                     &a.RunTests.DataDir,
		&a.OutDirDeprecated:                      &a.RunTests.OutDir,
		&a.TempDirDeprecated:                     &a.RunTests.TempDir,
		&a.CheckSoftwareDepsDeprecated:           &a.RunTests.CheckSoftwareDeps,
		&a.AvailableSoftwareFeaturesDeprecated:   &a.RunTests.AvailableSoftwareFeatures,
		&a.UnavailableSoftwareFeaturesDeprecated: &a.RunTests.UnavailableSoftwareFeatures,
		&a.WaitUntilReadyDeprecated:              &a.RunTests.WaitUntilReady,
	}
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by test runners to ensure that args will be interpreted
// correctly by older test bundles.
func (a *Args) FillDeprecated() {
	for old, cur := range a.deprecatedFields() {
		command.CopyFieldIfNonZero(cur, old)
	}
}

// RemoteArgs is nested within Args and holds additional arguments that are only relevant when running remote tests.
type RemoteArgs struct {
	// Target is the DUT connection spec as [<user>@]host[:<port>].
	Target string `json:"target,omitempty"`
	// KeyFile is the path to the SSH private key to use to connect to the DUT.
	KeyFile string `json:"keyFile,omitempty"`
	// KeyDir is the directory containing SSH private keys (typically $HOME/.ssh).
	KeyDir string `json:"keyDir,omitempty"`
	// TastPath contains the path to the tast binary that was executed to initiate testing.
	TastPath string `json:"tastPath,omitempty"`
	// RunFlags contains a subset of the flags that were passed to the "tast run" command.
	// The included flags are ones that are necessary for core functionality,
	// e.g. paths to binaries used by the tast process and credentials for reconnecting to the DUT.
	RunFlags []string `json:"runFlags,omitempty"`
}

// RunTestsArgs is nested within Args and contains additional arguments used by RunTestsMode.
type RunTestsArgs struct {
	// DataDir is the path to the directory containing test data files.
	DataDir string `json:"dataDir,omitempty"`
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string `json:"outDir,omitempty"`
	// TempDir is the path to the directory under which temporary files for tests are written.
	TempDir string `json:"tempDir,omitempty"`

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

// bundleType describes the type of tests contained in a test bundle (i.e. local or remote).
type bundleType int

const (
	localBundle bundleType = iota
	remoteBundle
)

// readArgs reads a JSON-marshaled Args struct from stdin and updates args (which may contain default values).
// Matched tests are returned. The caller is responsible for performing the requested action.
func readArgs(stdin io.Reader, args *Args, cfg *runConfig, bt bundleType) ([]*testing.Test, error) {
	if err := json.NewDecoder(stdin).Decode(args); err != nil {
		return nil, command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
	}
	if bt != remoteBundle && !reflect.DeepEqual(args.Remote, RemoteArgs{}) {
		return nil, command.NewStatusErrorf(statusBadArgs, "remote-only args %+v passed to non-remote bundle", args.Remote)
	}

	// Use non-zero-valued deprecated fields if they were supplied by an old test runner.
	for old, cur := range args.deprecatedFields() {
		command.CopyFieldIfNonZero(old, cur)
	}

	if errs := testing.RegistrationErrors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		return nil, command.NewStatusErrorf(statusBadTests, "error(s) in registered tests: %v", strings.Join(es, ", "))
	}

	tests, err := testsToRun(args.Patterns)
	if err != nil {
		return nil, command.NewStatusErrorf(statusBadPatterns, "failed getting tests for %v: %v", args.Patterns, err.Error())
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
	// TestPatternWildcard means the patterns will be interpreted as one or more wildcards (possibly literal test names).
	TestPatternWildcard TestPatternType = iota
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
		return TestPatternWildcard
	}
}

// testsToRun returns tests to run for a command invoked with test patterns pats.
// If no patterns are supplied, all registered tests are returned.
// If a single pattern is supplied and it is surrounded by parentheses,
// it is treated as a boolean expression specifying test attributes.
// Otherwise, pattern(s) are interpreted as wildcards matching test names.
func testsToRun(pats []string) ([]*testing.Test, error) {
	switch GetTestPatternType(pats) {
	case TestPatternWildcard:
		if len(pats) == 0 {
			return testing.GlobalRegistry().AllTests(), nil
		}
		// Print a helpful error message if it looks like the user wanted an attribute expression.
		if len(pats) == 1 && (strings.Contains(pats[0], "&&") || strings.Contains(pats[0], "||")) {
			return nil, fmt.Errorf("attr expr %q must be within parentheses", pats[0])
		}
		return testing.GlobalRegistry().TestsForPatterns(pats)
	case TestPatternAttrExpr:
		return testing.GlobalRegistry().TestsForAttrExpr(pats[0][1 : len(pats[0])-1])
	}
	return nil, fmt.Errorf("invalid test pattern(s) %v", pats)
}
