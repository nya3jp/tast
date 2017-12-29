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

// Log writes a RunLog control message to mw if non-nil or logs to stderr otherwise.
func Log(mw *control.MessageWriter, msg string) {
	if mw != nil {
		mw.WriteMessage(&control.RunLog{time.Now(), msg})
	} else {
		log.Print(msg)
	}
}

// Error writes a RunError control message to mw if non-nil or logs to stderr otherwise.
func Error(mw *control.MessageWriter, msg string) {
	if mw != nil {
		_, fn, ln, _ := runtime.Caller(1)
		mw.WriteMessage(&control.RunError{time.Now(), testing.Error{
			Reason: msg,
			File:   fn,
			Line:   ln,
			Stack:  string(debug.Stack()),
		}})
	} else {
		log.Print(msg)
	}
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
	tests []*testing.Test
	err   error
}

// getTests returns tests in bundles matched by patterns. It does this by executing
// each bundle with the -list arg to ask it to marshal and print its tests.
func getTests(bundles, patterns []string, dataDir string) ([]*testing.Test, error) {
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
				ch <- testsOrError{nil, err}
				return
			}
			tests := make([]*testing.Test, 0)
			if err := json.Unmarshal(out, &tests); err != nil {
				ch <- testsOrError{nil, fmt.Errorf("bundle %v gave bad output: %v", bundle, err)}
				return
			}
			ch <- testsOrError{tests, nil}
		}()
	}

	tests := make([]*testing.Test, 0)
	for i := 0; i < len(bundles); i++ {
		toe := <-ch
		if toe.err != nil {
			return nil, toe.err
		}
		tests = append(tests, toe.tests...)
	}
	return tests, nil
}

// ParseArgs parses args (typically os.Args[1:]) and returns a RunConfig if tests need to be run.
// defaultBundleGlob is a file glob matching bundles that should be executed.
// defaultDataDir is the default base directory containing test data.
// flags can be used to pass additional flag definitions; if it is non-nil, it will be modified.
// If the returned status is not 0, the caller should pass it to os.Exit.
// If the RunConfig is nil and the status is 0, the caller should exit with 0.
// If a non-nil RunConfig is returned, it should be passed to RunTests.
func ParseArgs(stdout io.Writer, args []string, defaultBundleGlob, defaultDataDir string,
	flags *flag.FlagSet) (*RunConfig, int) {
	if flags == nil {
		flags = flag.NewFlagSet("", flag.ContinueOnError)
	} else {
		flags.Init("", flag.ContinueOnError)
	}
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
			"Runs tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}

	cfg := RunConfig{stdout: stdout}
	bundleGlob := flags.String("bundles", defaultBundleGlob, "glob matching test bundles")
	report := flags.Bool("report", false, "report progress for calling process")
	listData := flags.Bool("listdata", false, "print data files needed for tests and exit")
	listTests := flags.Bool("listtests", false, "print matching tests and exit")
	flags.StringVar(&cfg.dataDir, "datadir", defaultDataDir, "directory containing data files")

	var err error
	if err = flags.Parse(args); err != nil {
		Error(nil, err.Error())
		return nil, statusBadArgs
	}
	if *report {
		cfg.mw = control.NewMessageWriter(stdout)
	}

	if cfg.bundles, err = getBundles(*bundleGlob); err != nil {
		Error(cfg.mw, fmt.Sprintf("Failed to get test bundle(s) %q: %v", *bundleGlob, err))
		return nil, statusNoBundles
	} else if len(cfg.bundles) == 0 {
		Error(cfg.mw, fmt.Sprintf("No test bundles matched by %q", *bundleGlob))
		return nil, statusNoBundles
	}

	cfg.patterns = flags.Args()
	if cfg.tests, err = getTests(cfg.bundles, cfg.patterns, cfg.dataDir); err != nil {
		Error(cfg.mw, fmt.Sprintf("Failed to get tests: %v", err.Error()))
		return nil, statusError
	}

	if *listData {
		if err = listDataFiles(stdout, cfg.tests); err != nil {
			Error(cfg.mw, fmt.Sprintf("Failed to list data files: %v", err))
			return nil, statusError
		}
		return nil, statusSuccess
	}
	if *listTests {
		if err = testing.WriteTestsAsJSON(stdout, cfg.tests); err != nil {
			Error(cfg.mw, fmt.Sprintf("Failed to write tests: %v", err))
			return nil, statusError
		}
		return nil, statusSuccess
	}

	return &cfg, statusSuccess
}

// RunConfig contains a configuration for running tests.
// Unexported fields are initialized by ParseArgs, but receivers may set exported fields
// before passing the configuration to RunTests.
type RunConfig struct {
	stdout  io.Writer              // location where bundle output should be copied
	mw      *control.MessageWriter // used to send control messages; nil if -report not passed
	dataDir string                 // base directory containing test data files

	bundles  []string // full paths of bundles to execute
	patterns []string // patterns matching tests to run

	// tests contains details of tests within bundles matched by patterns.
	// Note that these can't be executed directly, as they're just the unmarshaled structs
	// that the runner received from the bundles (i.e. their Func fields are nil).
	tests []*testing.Test

	// ExtraFlags contains extra flags to be passed to test bundles.
	ExtraFlags []string
	// PreRun is executed before the RunStart control message is written if non-nil and
	// if -report was passed.
	PreRun func(mw *control.MessageWriter)
	// PostRun is executed before the RunEnd control message is written if non-nil and
	// if -report was passed. Its return value is used as the base for the RunEnd message
	// that is sent to the tast command.
	PostRun func(mw *control.MessageWriter) control.RunEnd
}

// RunTests runs tests across multiple bundles as described by cfg.
// The returned status code should be passed to os.Exit.
func RunTests(cfg *RunConfig) int {
	if len(cfg.tests) == 0 {
		Error(cfg.mw, fmt.Sprintf("No tests matched by %v", cfg.patterns))
		return statusNoTests
	}

	outDir, err := ioutil.TempDir("", "tast_out.")
	if err != nil {
		Error(cfg.mw, fmt.Sprintf("Failed to create out dir: %v", err))
		return statusError
	}
	// If we have a MessageWriter because -report was passed, the tast command should clean up
	// the output dir after it copies it over. Otherwise, we should clean it up ourselves.
	if cfg.mw == nil {
		defer os.RemoveAll(outDir)
	}

	if cfg.mw != nil {
		if cfg.PreRun != nil {
			cfg.PreRun(cfg.mw)
		}
		cfg.mw.WriteMessage(&control.RunStart{time.Now(), len(cfg.tests)})
	}

	args := []string{"-datadir", cfg.dataDir, "-outdir", outDir}
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
			Error(cfg.mw, fmt.Sprintf("Bundle %v failed: %v (%v)", bundle, err, stderr.String()))
			return statusBundleFailed
		}
	}

	if cfg.mw != nil {
		var msg control.RunEnd
		if cfg.PostRun != nil {
			msg = cfg.PostRun(cfg.mw)
		}
		msg.Time = time.Now()
		msg.OutDir = outDir
		cfg.mw.WriteMessage(&msg)
	}

	return statusSuccess
}
