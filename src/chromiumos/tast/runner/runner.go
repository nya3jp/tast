// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

const (
	statusSuccess      = 0 // runner was successful
	statusError        = 1 // unspecified error was encountered
	statusBadArgs      = 2 // bad arguments were passed to the runner
	statusNoBundles    = 3 // glob passed to runner didn't match any bundles
	statusNoTests      = 4 // pattern(s) passed to runner didn't match any tests
	statusBundleFailed = 5 // test bundle exited with nonzero status
)

// logger is used to write messages to stdout when -report is not passed.
var logger *log.Logger = log.New(os.Stdout, "", log.LstdFlags)

// Log writes a RunLog control message to mw if non-nil or logs to stdout otherwise.
func Log(mw *control.MessageWriter, msg string) {
	if mw != nil {
		mw.WriteMessage(&control.RunLog{time.Now(), msg})
	} else {
		logger.Print(msg)
	}
}

// Error writes a RunError control message to mw if non-nil or writes the message
// directly to stderr otherwise. After calling this function, the runner should pass
// the returned status code (which may or may not be equal to the status arg) to os.Exit.
func Error(mw *control.MessageWriter, msg string, status int) int {
	if mw == nil {
		fmt.Fprintln(os.Stderr, msg)
		return status
	}

	_, fn, ln, _ := runtime.Caller(1)
	mw.WriteMessage(&control.RunError{time.Now(), testing.Error{
		Reason: msg,
		File:   fn,
		Line:   ln,
		Stack:  string(debug.Stack()),
	}})
	// Exit with success when reporting progress via control messages.
	// The tast command will know that the run failed because of the RunError message.
	return statusSuccess
}

// getBundles returns the full paths of all test bundles matched by glob.
func getBundles(glob string) ([]string, error) {
	ps, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}

	bundles := make([]string, 0)
	for _, p := range ps {
		fi, err := os.Stat(p)
		// Only match executable regular files.
		if err == nil && fi.Mode().IsRegular() && (fi.Mode().Perm()&0111) != 0 {
			bundles = append(bundles, p)
		}
	}
	return bundles, nil
}

type testsOrError struct {
	bundle string
	tests  []*testing.Test
	err    error
}

// getTests returns tests in bundles matched by patterns. It does this by executing
// each bundle with the -list arg to ask it to marshal and print its tests. A slice
// of paths to bundles with matched tests is also returned.
func getTests(bundles, patterns []string, dataDir string) (
	tests []*testing.Test, bundlesWithTests []string, err error) {
	args := []string{"-datadir", dataDir, "-list"}
	args = append(args, patterns...)

	// Run all bundles in parallel.
	ch := make(chan testsOrError, len(bundles))
	for _, b := range bundles {
		bundle := b
		go func() {
			out, err := exec.Command(bundle, args...).Output()
			if err != nil {
				// Pass back stderr if the command reported an error.
				if ee, ok := err.(*exec.ExitError); ok {
					err = fmt.Errorf("bundle %v failed: %v", bundle, string(ee.Stderr))
				}
				ch <- testsOrError{bundle, nil, err}
				return
			}
			ts := make([]*testing.Test, 0)
			if err := json.Unmarshal(out, &ts); err != nil {
				ch <- testsOrError{bundle, nil,
					fmt.Errorf("bundle %v gave bad output: %v", bundle, err)}
				return
			}
			ch <- testsOrError{bundle, ts, nil}
		}()
	}

	tests = make([]*testing.Test, 0)
	for i := 0; i < len(bundles); i++ {
		toe := <-ch
		if toe.err != nil {
			return nil, nil, toe.err
		}
		if len(toe.tests) > 0 {
			tests = append(tests, toe.tests...)
			bundlesWithTests = append(bundlesWithTests, toe.bundle)
		}
	}
	return tests, bundlesWithTests, nil
}

// RunnerType describes the type of test runner that is using this package.
type RunnerType int

const (
	// LocalRunner indicates that this package is being used by local_test_runner.
	LocalRunner RunnerType = iota
	// RemoteRunner indicates that this package is being used by remote_test_runner.
	RemoteRunner
)

// ParseArgs parses runtime arguments and returns a RunConfig if tests need to be run.
//
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is populated both by parsing clArgs and
// (if -report is passed) by decoding an JSON-marshaled Args struct from stdin.
//
// If the returned status is not 0, the caller should pass it to os.Exit.
// If the RunConfig is nil and the status is 0, the caller should exit with 0.
// If a non-nil RunConfig is returned, it should be passed to RunTests.
func ParseArgs(clArgs []string, stdin io.Reader, stdout io.Writer, args *Args, runnerType RunnerType) (*RunConfig, int) {
	cfg := RunConfig{stdout: stdout}

	if len(clArgs) == 0 {
		cfg.mw = control.NewMessageWriter(stdout)
		if err := json.NewDecoder(stdin).Decode(args); err != nil {
			return nil, statusBadArgs
		}
	} else {
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
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
		// TODO(derat): Remove -report, -listdata, and -listtests once the tast command always writes args to stdin.
		report := flags.Bool("report", false, "read args from stdin and report progress for tast command")
		listData := flags.Bool("listdata", false, "print data files needed for tests and exit")
		listTests := flags.Bool("listtests", false, "print matching tests and exit")

		if err := flags.Parse(clArgs); err != nil {
			return nil, statusBadArgs
		}

		if *report {
			cfg.mw = control.NewMessageWriter(stdout)
		}
		if *listData {
			args.Mode = ListDataMode
		} else if *listTests {
			args.Mode = ListTestsMode
		}
		args.Patterns = flags.Args()
	}

	if runnerType != RemoteRunner && args.RemoteArgs != (RemoteArgs{}) {
		return nil, Error(cfg.mw, fmt.Sprintf("Remote args %v passed to non-remote runner", args.RemoteArgs), statusBadArgs)
	}

	cfg.dataDir = args.DataDir
	cfg.outDir = args.OutDir
	cfg.patterns = args.Patterns

	var err error
	if cfg.bundles, err = getBundles(args.BundleGlob); err != nil {
		return nil, Error(cfg.mw, fmt.Sprintf("Failed to get test bundle(s) %q: %v", args.BundleGlob, err), statusNoBundles)
	} else if len(cfg.bundles) == 0 {
		return nil, Error(cfg.mw, fmt.Sprintf("No test bundles matched by %q", args.BundleGlob), statusNoBundles)
	}
	if cfg.tests, cfg.bundles, err = getTests(cfg.bundles, cfg.patterns, cfg.dataDir); err != nil {
		return nil, Error(cfg.mw, fmt.Sprintf("Failed to get tests: %v", err.Error()), statusError)
	}

	if args.Mode == ListDataMode {
		if err = listDataFiles(stdout, cfg.tests); err != nil {
			return nil, Error(cfg.mw, fmt.Sprintf("Failed to list data files: %v", err), statusError)
		}
		return nil, statusSuccess
	} else if args.Mode == ListTestsMode {
		if err = testing.WriteTestsAsJSON(stdout, cfg.tests); err != nil {
			return nil, Error(cfg.mw, fmt.Sprintf("Failed to write tests: %v", err), statusError)
		}
		return nil, statusSuccess
	}
	return &cfg, statusSuccess
}

// RunMode describes the runner's behavior.
type RunMode int

const (
	// RunTestsMode indicates that the runner should run all matched tests.
	RunTestsMode RunMode = iota
	// ListDataMode indicates that the runner should write data files used by matched tests to stdout as a
	// JSON array of strings and exit.
	// TODO(derat): Deprecate this value and remove the supporting code. ListTestsMode already includes data
	// files, so tast should just use that instead.
	ListDataMode
	// ListTestsMode indicates that the runner should write information about matched tests to stdout as a
	// JSON array of testing.Test structs and exit.
	ListTestsMode
)

// Args provides a backward- and forward-compatible way to pass arguments from tast executable to test runners.
// The tast executable writes the struct's JSON-serialized representation to the runner's stdin.
type Args struct {
	// Mode describes the mode that should be used by the runner.
	Mode RunMode `json:"mode"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob"`
	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns"`
	// DataDir is the path to the directory containing test data files.
	DataDir string `json:"dataDir"`
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string `json:"outDir"`
	// RemoteArgs contains additional arguments used to run remote tests.
	RemoteArgs
}

// RemoteArgs is nested within Args and holds additional arguments that are only relevant when running
// remote tests.
type RemoteArgs struct {
	// Target is the DUT connection spec as [<user>@]host[:<port>].
	Target string `json:"remoteTarget"`
	// KeyFile is the path to the SSH private key to use to connect to the DUT.
	KeyFile string `json:"remoteKeyFile"`
	// KeyDir is the directory containing SSH private keys (typically $HOME/.ssh).
	KeyDir string `json:"remoteKeyDir"`
}

// RunConfig contains a configuration for running tests.
// Unexported fields are initialized by ParseArgs, but receivers may set exported fields
// before passing the configuration to RunTests.
type RunConfig struct {
	stdout  io.Writer              // location where bundle output should be copied
	mw      *control.MessageWriter // used to send control messages to tast command; nil for manual run
	dataDir string                 // base directory containing test data files
	outDir  string                 // base directory to write output files to

	bundles  []string // full paths of bundles to execute
	patterns []string // patterns matching tests to run

	// tests contains details of tests within bundles matched by patterns.
	// Note that these can't be executed directly, as they're just the unmarshaled structs
	// that the runner received from the bundles (i.e. their Func fields are nil).
	tests []*testing.Test

	// ExtraFlags contains extra flags to be passed to test bundles.
	ExtraFlags []string
	// PreRun is executed before the RunStart control message is written if it and mw are non-nil
	// and one or more tests were matched.
	PreRun func(mw *control.MessageWriter)
	// PostRun is executed before the RunEnd control message is written if it and mw are non-nil
	// and one or more tests were matched. Its return value is used as the base for the
	// RunEnd message that is sent to the tast command.
	PostRun func(mw *control.MessageWriter) control.RunEnd
}

// RunTests runs tests across multiple bundles as described by cfg.
// The returned status code should be passed to os.Exit.
func RunTests(cfg *RunConfig) int {
	if len(cfg.tests) == 0 {
		// If the runner was executed manually, report an error if no tests were matched.
		if cfg.mw == nil {
			return Error(nil, fmt.Sprintf("No tests matched by %v", cfg.patterns), statusNoTests)
		}

		// Otherwise, just report an empty run. It's expected to not match any tests if
		// both local and remote tests are being run but the user specified a pattern that
		// matched only local or only remote tests rather than tests of both types.
		cfg.mw.WriteMessage(&control.RunStart{Time: time.Now()})
		cfg.mw.WriteMessage(&control.RunEnd{Time: time.Now()})
		return statusSuccess
	}

	if cfg.outDir == "" {
		var err error
		if cfg.outDir, err = ioutil.TempDir("", "tast_out."); err != nil {
			return Error(cfg.mw, fmt.Sprintf("Failed to create out dir: %v", err), statusError)
		}
		// If we were run by the tast command, it should clean up the output dir after it copies it over.
		// Otherwise, we should clean it up ourselves.
		if cfg.mw == nil {
			defer os.RemoveAll(cfg.outDir)
		}
	}

	if cfg.mw != nil {
		if cfg.PreRun != nil {
			cfg.PreRun(cfg.mw)
		}
		cfg.mw.WriteMessage(&control.RunStart{time.Now(), len(cfg.tests)})
	}

	args := []string{"-datadir", cfg.dataDir, "-outdir", cfg.outDir}
	if cfg.mw != nil {
		args = append(args, "-report")
	}
	args = append(args, cfg.ExtraFlags...)
	args = append(args, cfg.patterns...)

	// Execute bundles serially to run tests.
	for _, bundle := range cfg.bundles {
		cmd := exec.Command(bundle, args...)
		cmd.Stdout = cfg.stdout
		stderr := bytes.Buffer{}
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return Error(cfg.mw, fmt.Sprintf("Bundle %v failed: %v (%v)", bundle, err, stderr.String()),
				statusBundleFailed)
		}
	}

	if cfg.mw != nil {
		var msg control.RunEnd
		if cfg.PostRun != nil {
			msg = cfg.PostRun(cfg.mw)
		}
		msg.Time = time.Now()
		msg.OutDir = cfg.outDir
		cfg.mw.WriteMessage(&msg)
	}

	return statusSuccess
}
