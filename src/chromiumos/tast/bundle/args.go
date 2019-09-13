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
	"time"

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
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by test runners to ensure that args will be interpreted
// correctly by older test bundles.
func (a *Args) FillDeprecated() {
	// If there were any deprecated fields, we would fill them from the corresponding
	// non-deprecated fields here using command.CopyFieldIfNonZero for basic types or
	// manual copies for structs.
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
	// We don't have any deprecated fields right now.
}

// RunTestsArgs is nested within Args and contains arguments used by RunTestsMode.
type RunTestsArgs struct {
	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns,omitempty"`

	// TestVars contains names and values of runtime variables used to pass out-of-band data to tests.
	// Names correspond to testing.Test.Vars and values are accessed using testing.State.Var.
	TestVars map[string]string `json:"testVars,omitempty"`

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
	// HeartbeatInterval is the interval in seconds at which heartbeat messages are sent back
	// periodically from runners (before running bundles) and bundles. If this value is not
	// positive, heartbeat messages are not sent.
	HeartbeatInterval time.Duration `json:"heartbeatInterval,omitempty"`
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
	args *Args, cfg *runConfig, bt bundleType) ([]*testing.TestCase, error) {
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

	if (args.Mode == RunTestsMode && args.RunTests == nil) ||
		(args.Mode == ListTestsMode && args.ListTests == nil) {
		return nil, command.NewStatusErrorf(statusBadArgs, "args not set for mode %v", args.Mode)
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

	tests, err := testing.SelectTestsByArgs(testing.GlobalRegistry().AllTests(), patterns)
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
