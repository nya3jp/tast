// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"chromiumos/tast/control"
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
	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns,omitempty"`
	// DataDir is the path to the directory containing test data files.
	DataDir string `json:"dataDir,omitempty"`
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string `json:"outDir,omitempty"`

	// RemoteArgs contains additional arguments used to run remote tests.
	RemoteArgs
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

// bundleType describes the type of tests contained in a test bundle (i.e. local or remote).
type bundleType int

const (
	localBundle bundleType = iota
	remoteBundle
)

// readArgs reads a JSON-marshaled Args struct from stdin and returns a runConfig if tests need to be run.
// args contains default values for arguments and is further populated from stdin.
// If the returned status is not statusSuccess, the caller should pass it to os.Exit.
// If the runConfig is nil and the status is statusSuccess, the caller should exit with 0.
// If a non-nil runConfig is returned, it should be passed to runTests.
// TODO(derat): Refactor this code to not have such tricky multi-modal behavior around either
// returning a config that should be passed to runTests or listing tests directly.
func readArgs(stdin io.Reader, stdout io.Writer, args *Args, bt bundleType) (*runConfig, int) {
	if err := json.NewDecoder(stdin).Decode(args); err != nil {
		writeError("Failed to decode args from stdin")
		return nil, statusBadArgs
	}
	if bt != remoteBundle && args.RemoteArgs != (RemoteArgs{}) {
		writeError(fmt.Sprintf("Remote-only args %+v passed to non-remote bundle", args.RemoteArgs))
		return nil, statusBadArgs
	}
	if errs := testing.RegistrationErrors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		writeError("Error(s) in registered tests: " + strings.Join(es, "\n"))
		return nil, statusBadTests
	}

	cfg := runConfig{mw: control.NewMessageWriter(stdout), args: args}
	var err error
	if cfg.tests, err = testsToRun(args.Patterns); err != nil {
		writeError(fmt.Sprintf("Failed getting tests for %v: %v", args.Patterns, err.Error()))
		return nil, statusBadPatterns
	}
	sort.Slice(cfg.tests, func(i, j int) bool { return cfg.tests[i].Name < cfg.tests[j].Name })

	switch args.Mode {
	case ListTestsMode:
		if err = testing.WriteTestsAsJSON(stdout, cfg.tests); err != nil {
			writeError(err.Error())
			return nil, statusError
		}
		return nil, statusSuccess
	case RunTestsMode:
		return &cfg, statusSuccess
	default:
		writeError(fmt.Sprintf("Invalid mode %v", args.Mode))
		return nil, statusBadArgs
	}
}

// testsToRun returns tests to run for a command invoked with test patterns pats.
// If no patterns are supplied, all registered tests are returned.
// If a single pattern is supplied and it is surrounded by parentheses,
// it is treated as a boolean expression specifying test attributes.
// Otherwise, pattern(s) are interpreted as wildcards matching test names.
func testsToRun(pats []string) ([]*testing.Test, error) {
	if len(pats) == 0 {
		return testing.GlobalRegistry().AllTests(), nil
	}
	if len(pats) == 1 && strings.HasPrefix(pats[0], "(") && strings.HasSuffix(pats[0], ")") {
		return testing.GlobalRegistry().TestsForAttrExpr(pats[0][1 : len(pats[0])-1])
	}
	// Print a helpful error message if it looks like the user wanted an attribute expression.
	if len(pats) == 1 && (strings.Contains(pats[0], "&&") || strings.Contains(pats[0], "||")) {
		return nil, fmt.Errorf("attr expr %q must be within parentheses", pats[0])
	}
	return testing.GlobalRegistry().TestsForPatterns(pats)
}
