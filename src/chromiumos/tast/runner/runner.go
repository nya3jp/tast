// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"sort"
	"time"

	"chromiumos/tast/bundle"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

// TODO(derat): Split this into separate args.go and run.go files.

const (
	statusSuccess      = 0 // runner was successful
	statusError        = 1 // unspecified error was encountered
	statusBadArgs      = 2 // bad arguments were passed to the runner
	statusNoBundles    = 3 // glob passed to runner didn't match any bundles
	statusNoTests      = 4 // pattern(s) passed to runner didn't match any tests
	statusBundleFailed = 5 // test bundle exited with nonzero status
)

// logger is used to write messages to stdout when the runner is executed manually
// rather than by the tast command.
var logger *log.Logger = log.New(os.Stdout, "", log.LstdFlags)

// Log writes a RunLog control message to mw if non-nil or logs to stdout otherwise.
func Log(mw *control.MessageWriter, msg string) {
	if mw != nil {
		mw.WriteMessage(&control.RunLog{Time: time.Now(), Text: msg})
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
	mw.WriteMessage(&control.RunError{Time: time.Now(), Error: testing.Error{
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
	sort.Strings(bundles)
	return bundles, nil
}

// runBundle runs the bundle at path to completion, passing args.
// The bundle's stdout is copied to the stdout arg.
func runBundle(path string, args *bundle.Args, stdout io.Writer) error {
	cmd := exec.Command(path)
	cmd.Stdout = stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}

	jerr := json.NewEncoder(stdin).Encode(args)
	stdin.Close()
	err = cmd.Wait()

	if jerr != nil {
		return jerr
	}
	if err != nil {
		// Pass back stderr if the command reported an error.
		if ee, ok := err.(*exec.ExitError); ok {
			return errors.New(string(ee.Stderr))
		}
	}
	return err
}

type testsOrError struct {
	bundle string
	tests  []*testing.Test
	err    error
}

// getTests returns tests in bundles matched by patterns. It does this by executing
// each bundle to ask it to marshal and print its tests. A slice of paths to bundles
// with matched tests is also returned.
func getTests(bundles []string, baseArgs *bundle.Args) (
	tests []*testing.Test, bundlesWithTests []string, err error) {
	args := *baseArgs
	args.Mode = bundle.ListTestsMode

	// Run all bundles in parallel.
	ch := make(chan testsOrError, len(bundles))
	for _, b := range bundles {
		bundle := b
		go func() {
			out := bytes.Buffer{}
			if err := runBundle(bundle, &args, &out); err != nil {
				ch <- testsOrError{bundle, nil, fmt.Errorf("bundle %v failed: %v", bundle, err)}
				return
			}
			ts := make([]*testing.Test, 0)
			if err := json.Unmarshal(out.Bytes(), &ts); err != nil {
				ch <- testsOrError{bundle, nil,
					fmt.Errorf("bundle %v gave bad output: %v", bundle, err)}
				return
			}
			ch <- testsOrError{bundle, ts, nil}
		}()
	}

	// Read results into a map from bundle to that bundle's tests.
	bundleTests := make(map[string][]*testing.Test)
	for i := 0; i < len(bundles); i++ {
		toe := <-ch
		if toe.err != nil {
			return nil, nil, toe.err
		}
		if len(toe.tests) > 0 {
			bundleTests[toe.bundle] = toe.tests
		}
	}

	// Sort both the tests and the bundles by bundle path.
	for b := range bundleTests {
		bundlesWithTests = append(bundlesWithTests, b)
	}
	sort.Strings(bundlesWithTests)
	for _, b := range bundlesWithTests {
		tests = append(tests, bundleTests[b]...)
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
// args contains default values for arguments and is further populated by parsing clArgs or
// (if clArgs is empty, as is the case when a runner is executed by the tast command) by
// decoding a JSON-marshaled Args struct from stdin.
//
// If the returned status is not 0, the caller should pass it to os.Exit.
// If the RunConfig is nil and the status is 0, the caller should exit with 0.
// If a non-nil RunConfig is returned, it should be passed to RunTests.
// TODO(derat): Refactor this code to not have such tricky multi-modal behavior around either
// returning a config that should be passed to RunTests or listing tests directly.
func ParseArgs(clArgs []string, stdin io.Reader, stdout io.Writer, args *Args, runnerType RunnerType) (*RunConfig, int) {
	var mw *control.MessageWriter

	if len(clArgs) == 0 {
		if err := json.NewDecoder(stdin).Decode(args); err != nil {
			return nil, statusBadArgs
		}
		// Control messages are only used when performing a test run. Otherwise, errors should go to stderr.
		if args.Mode == RunTestsMode {
			mw = control.NewMessageWriter(stdout)
		}
	} else {
		// Expose a limited amount of configurability via command-line flags to support running test runners manually.
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

		if err := flags.Parse(clArgs); err != nil {
			return nil, statusBadArgs
		}
		args.Mode = RunTestsMode
		args.Patterns = flags.Args()
	}

	// Handle system-info-related commands first; they don't require touching test bundles.
	switch args.Mode {
	case GetSysInfoStateMode:
		if err := handleGetSysInfoState(args, stdout); err != nil {
			return nil, Error(nil, err.Error(), statusError)
		}
		return nil, statusSuccess
	case CollectSysInfoMode:
		if err := handleCollectSysInfo(args, stdout); err != nil {
			return nil, Error(nil, err.Error(), statusError)
		}
		return nil, statusSuccess
	}

	cfg := RunConfig{
		stdout: stdout,
		mw:     mw,
		bundleArgs: bundle.Args{
			DataDir:  args.DataDir,
			OutDir:   args.OutDir,
			Patterns: args.Patterns,
		},
	}

	if args.RemoteArgs != (RemoteArgs{}) {
		if runnerType == RemoteRunner {
			cfg.bundleArgs.RemoteArgs = bundle.RemoteArgs{
				Target:  args.Target,
				KeyFile: args.KeyFile,
				KeyDir:  args.KeyDir,
			}
		} else {
			return nil, Error(cfg.mw, fmt.Sprintf("Remote args %v passed to non-remote runner", args.RemoteArgs), statusBadArgs)
		}
	}

	var err error
	if cfg.bundles, err = getBundles(args.BundleGlob); err != nil {
		return nil, Error(cfg.mw, fmt.Sprintf("Failed to get test bundle(s) %q: %v", args.BundleGlob, err), statusNoBundles)
	} else if len(cfg.bundles) == 0 {
		return nil, Error(cfg.mw, fmt.Sprintf("No test bundles matched by %q", args.BundleGlob), statusNoBundles)
	}
	if cfg.tests, cfg.bundles, err = getTests(cfg.bundles, &cfg.bundleArgs); err != nil {
		return nil, Error(cfg.mw, fmt.Sprintf("Failed to get tests: %v", err.Error()), statusError)
	}

	switch args.Mode {
	case RunTestsMode:
		return &cfg, statusSuccess
	case ListTestsMode:
		if err = testing.WriteTestsAsJSON(stdout, cfg.tests); err != nil {
			return nil, Error(cfg.mw, fmt.Sprintf("Failed to write tests: %v", err), statusError)
		}
		return nil, statusSuccess
	default:
		return nil, Error(cfg.mw, fmt.Sprintf("Invalid mode %v", args.Mode), statusBadArgs)
	}
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

// RunConfig contains a configuration for running tests.
// It is returned by ParseArgs and passed to RunTests.
type RunConfig struct {
	stdout     io.Writer              // location where bundle output should be copied
	mw         *control.MessageWriter // used to send control messages to tast command; nil for manual run
	bundles    []string               // full paths of bundles to execute
	bundleArgs bundle.Args            // args to pass to bundles

	// tests contains details of tests within bundles matched by patterns.
	// Note that these can't be executed directly, as they're just the unmarshaled structs
	// that the runner received from the bundles (i.e. their Func fields are nil).
	tests []*testing.Test
}

// RunTests runs tests across multiple bundles as described by cfg.
// The returned status code should be passed to os.Exit.
func RunTests(cfg *RunConfig) int {
	if len(cfg.tests) == 0 {
		// If the runner was executed manually, report an error if no tests were matched.
		if cfg.mw == nil {
			return Error(nil, fmt.Sprintf("No tests matched by %v", cfg.bundleArgs.Patterns), statusNoTests)
		}

		// Otherwise, just report an empty run. It's expected to not match any tests if
		// both local and remote tests are being run but the user specified a pattern that
		// matched only local or only remote tests rather than tests of both types.
		cfg.mw.WriteMessage(&control.RunStart{Time: time.Now()})
		cfg.mw.WriteMessage(&control.RunEnd{Time: time.Now()})
		return statusSuccess
	}

	if cfg.bundleArgs.OutDir == "" {
		var err error
		if cfg.bundleArgs.OutDir, err = ioutil.TempDir("", "tast_out."); err != nil {
			return Error(cfg.mw, fmt.Sprintf("Failed to create out dir: %v", err), statusError)
		}
		// If we were run by the tast command, it should clean up the output dir after it copies it over.
		// Otherwise, we should clean it up ourselves.
		if cfg.mw == nil {
			defer os.RemoveAll(cfg.bundleArgs.OutDir)
		}
	}

	if cfg.mw != nil {
		cfg.mw.WriteMessage(&control.RunStart{Time: time.Now(), NumTests: len(cfg.tests)})
	}

	// Execute bundles serially to run tests.
	cfg.bundleArgs.Mode = bundle.RunTestsMode
	for _, bundle := range cfg.bundles {
		if err := runBundleTests(bundle, cfg); err != nil {
			return Error(cfg.mw, fmt.Sprintf("Bundle %v failed: %v", bundle, err), statusBundleFailed)
		}
	}

	if cfg.mw != nil {
		cfg.mw.WriteMessage(&control.RunEnd{Time: time.Now(), OutDir: cfg.bundleArgs.OutDir})
	}

	return statusSuccess
}

// runBundleTests executes tests in the bundle at path bundle as instructed by cfg.
func runBundleTests(bundle string, cfg *RunConfig) error {
	// When we were run by the tast command, just copy stdout over directly.
	if cfg.mw != nil {
		return runBundle(bundle, &cfg.bundleArgs, cfg.stdout)
	}

	// When we were run manually, read control messages from stdout so we can log
	// them in a human-readable form.

	// First, start a goroutine to log messages as they're produced by the bundle.
	pr, pw := io.Pipe()
	ch := make(chan error, 1)
	go func() { ch <- logBundleOutputForManualRun(pr) }()

	// Run the bundle to completion, copying its output to the goroutine over the pipe.
	if err := runBundle(bundle, &cfg.bundleArgs, pw); err != nil {
		pw.Close()
		return err
	}
	pw.Close()

	// Finally, wait for the goroutine to finish and return its result.
	return <-ch
}

// logBundleOutputForManualRun reads control messages from src and logs them to stdout.
// It is used to print human-readable test output when the runner is executed manually rather
// than via the tast command. An error is returned if any TestError messages are read from src.
func logBundleOutputForManualRun(src io.Reader) error {
	failed := false
	mr := control.NewMessageReader(src)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			return err
		}
		switch v := msg.(type) {
		case *control.TestStart:
			logger.Print("Running ", v.Test.Name)
		case *control.TestLog:
			logger.Print(v.Text)
		case *control.TestError:
			logger.Printf("Error: [%s:%d] %v", v.Error.File, v.Error.Line, v.Error.Reason)
			failed = true
		case *control.TestEnd:
			logger.Print("Finished ", v.Name)
		}
	}

	if failed {
		return errors.New("Test(s) failed")
	}
	return nil
}
