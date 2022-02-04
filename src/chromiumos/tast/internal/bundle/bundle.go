// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle/legacyjson"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
)

const (
	statusSuccess     = 0 // bundle ran successfully
	statusError       = 1 // unclassified runtime error was encountered
	statusBadArgs     = 2 // bad command-line flags or other args were supplied
	statusBadTests    = 3 // errors in test registration (bad names, missing test functions, etc.)
	statusBadPatterns = 4 // one or more bad test patterns were passed to the bundle
	_                 = 5 // deprecated
)

// Delegate injects functions as a part of test bundle framework implementation.
type Delegate struct {
	// TestHook is called before each test in the test bundle if it is not nil.
	// The returned closure is executed after the test if it is not nil.
	TestHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)

	// RunHook is called at the beginning of a bundle execution if it is not nil.
	// The returned closure is executed at the end if it is not nil.
	// In case of errors, no test in the test bundle will run.
	RunHook func(context.Context) (func(context.Context) error, error)

	// Ready is called at the beginning of a bundle execution if it is not
	// nil and -waituntilready is set to true (default).
	// systemTestsTimeout is the timeout for waiting for system services
	// to be ready in seconds.
	// Local test bundles can specify a function to wait for the DUT to be
	// ready for tests to run. It is recommended to write informational
	// messages with testing.ContextLog to let the user know the reason for
	// the delay. In case of errors, no test in the test bundle will run.
	// This field has an effect only for local test bundles.
	Ready func(ctx context.Context, systemTestsTimeout time.Duration) error

	// BeforeReboot is called before every reboot if it is not nil.
	// This field has an effect only for remote test bundles.
	BeforeReboot func(ctx context.Context, d *dut.DUT) error

	// BeforeDownload is called before the framework attempts to download
	// external data files if it is not nil.
	//
	// Test bundles can install this hook to recover from possible network
	// outage caused by previous tests. Note that it is called only when
	// the framework needs to download one or more external data files.
	//
	// Since no specific timeout is set to ctx, do remember to set a
	// reasonable timeout at the beginning of the hook to avoid blocking
	// for long time.
	BeforeDownload func(ctx context.Context)
}

// run reads a JSON-marshaled BundleArgs struct from stdin and performs the requested action.
// Default arguments may be specified via args, which will also be updated from stdin.
// The caller should exit with the returned status code.
func run(ctx context.Context, clArgs []string, stdin io.Reader, stdout, stderr io.Writer, scfg *StaticConfig) int {
	args, err := readArgs(clArgs, stderr)
	if err != nil {
		return command.WriteError(stderr, err)
	}

	if errs := scfg.registry.Errors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		err := command.NewStatusErrorf(statusBadTests, "error(s) in registered tests: %v", strings.Join(es, ", "))
		return command.WriteError(stderr, err)
	}

	switch args.mode {
	case modeDumpTests:
		tests, err := testsToRun(scfg, nil)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		switch args.dumpFormat {
		case dumpFormatLegacyJSON:
			if err := writeTestsAsLegacyJSON(stdout, tests); err != nil {
				return command.WriteError(stderr, err)
			}
			return statusSuccess
		case dumpFormatProto:
			if err := testing.WriteTestsAsProto(stdout, tests); err != nil {
				return command.WriteError(stderr, err)
			}
		default:
			return command.WriteError(stderr, errors.Errorf("invalid dump format %v", args.dumpFormat))
		}
		return statusSuccess
	case modeRPC:
		if err := RunRPCServer(stdin, stdout, scfg); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case modeRPCTCP:
		port := args.port
		handshakeReq := args.handshake
		if err := RunRPCServerTCP(port, handshakeReq, scfg); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.mode))
	}
}

func writeTestsAsLegacyJSON(w io.Writer, tests []*testing.TestInstance) error {
	var infos []*legacyjson.EntityWithRunnabilityInfo
	for _, test := range tests {
		// If we encounter errors while checking test dependencies,
		// treat the test as not skipped. When we actually try to
		// run the test later, it will fail with errors.
		var skipReason string
		if reasons, err := test.Deps().Check(nil); err == nil && len(reasons) > 0 {
			skipReason = strings.Join(append([]string(nil), reasons...), ", ")
		}
		infos = append(infos, &legacyjson.EntityWithRunnabilityInfo{
			EntityInfo: *legacyjson.MustEntityInfoFromProto(test.EntityProto()),
			SkipReason: skipReason,
		})
	}
	return json.NewEncoder(w).Encode(infos)
}

// StaticConfig contains configurations unique to a test bundle.
//
// The supplied functions are used to provide customizations that apply to all
// entities in a test bundle. They may contain bundle-specific code.
type StaticConfig struct {
	// registry is a registry to be used to find entities.
	registry *testing.Registry
	// runHook is run at the beginning of the entire series of tests if non-nil.
	// The returned closure is executed after the entire series of tests if not nil.
	runHook func(context.Context, time.Duration) (func(context.Context) error, error)
	// testHook is run before each test if non-nil.
	// If this function panics or reports errors, the precondition (if any)
	// will not be prepared and the test function will not run.
	// The returned closure is executed after a test if not nil.
	testHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)
	// beforeReboot is run before every reboot if non-nil.
	// The function must not call DUT.Reboot() or it will cause infinite recursion.
	beforeReboot func(context.Context, *dut.DUT) error
	// beforeDownload is run before downloading external data files if non-nil.
	beforeDownload func(context.Context)
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

// NewStaticConfig constructs StaticConfig from given parameters.
func NewStaticConfig(reg *testing.Registry, defaultTestTimeout time.Duration, d Delegate) *StaticConfig {
	return &StaticConfig{
		registry: reg,
		runHook: func(ctx context.Context, systemTestsTimeout time.Duration) (func(context.Context) error, error) {
			pd, ok := testcontext.PrivateDataFromContext(ctx)
			if !ok {
				panic("BUG: PrivateData not available in run hook")
			}
			if d.Ready != nil && pd.WaitUntilReady {
				if err := d.Ready(ctx, systemTestsTimeout); err != nil {
					return nil, err
				}
			}
			if d.RunHook == nil {
				return nil, nil
			}
			return d.RunHook(ctx)
		},
		testHook:           d.TestHook,
		beforeReboot:       d.BeforeReboot,
		beforeDownload:     d.BeforeDownload,
		defaultTestTimeout: defaultTestTimeout,
	}
}
