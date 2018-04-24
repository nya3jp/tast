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

// readArgs reads a JSON-marshaled Args struct from stdin and updates args (which may contain default values).
// Matched tests are returned. The caller is responsible for performing the requested action.
func readArgs(stdin io.Reader, args *Args, bt bundleType) ([]*testing.Test, error) {
	if err := json.NewDecoder(stdin).Decode(args); err != nil {
		return nil, newBundleErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
	}
	if bt != remoteBundle && args.RemoteArgs != (RemoteArgs{}) {
		return nil, newBundleErrorf(statusBadArgs, "remote-only args %+v passed to non-remote bundle", args.RemoteArgs)
	}
	if errs := testing.RegistrationErrors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		return nil, newBundleErrorf(statusBadTests, "error(s) in registered tests: %v", strings.Join(es, ", "))
	}

	tests, err := testsToRun(args.Patterns)
	if err != nil {
		return nil, newBundleErrorf(statusBadPatterns, "failed getting tests for %v: %v", args.Patterns, err.Error())
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].Name < tests[j].Name })
	return tests, nil
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
