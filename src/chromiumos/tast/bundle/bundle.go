// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

const (
	statusSuccess     = 0 // bundle ran successfully
	statusError       = 1 // unclassified runtime error was encountered
	statusBadArgs     = 2 // bad command-line flags or other args were supplied
	statusBadTests    = 3 // errors in test registration (bad names, missing test functions, etc.)
	statusBadPatterns = 4 // one or more bad test patterns were passed to the bundle
	statusTestsFailed = 5 // one or more tests failed while running when -report not passed
	statusNoTests     = 6 // no tests were matched by the supplied patterns

	// Number of characters in prefixes from the log package, e.g. "2017/08/17 09:29:54 ".
	logPrefixLen = 20
)

// logger is used to write messages to stdout when -report is not passed.
var logger *log.Logger = log.New(os.Stdout, "", log.LstdFlags)

// writeError writes an error to stderr.
func writeError(msg string) {
	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		msg += "\n"
	}
	io.WriteString(os.Stderr, msg)
}

// parseArgs parses args (typically os.Args[1:]) and returns a runConfig if tests need to be run.
// defaultDataDir is the default base directory containing test data.
// flags can be used to pass additional flag definitions; if it is non-nil, it will be modified.
// If the returned status is not statusSuccess, the caller should pass it to os.Exit.
// If the runConfig is nil and the status is statusSuccess, the caller should exit with 0.
// If a non-nil runConfig is returned, it should be passed to runTests.
func parseArgs(stdout io.Writer, args []string, defaultDataDir string,
	flags *flag.FlagSet) (*runConfig, int) {
	if flags == nil {
		flags = flag.NewFlagSet("", flag.ContinueOnError)
	} else {
		flags.Init("", flag.ContinueOnError)
	}
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <flags> <pattern> <pattern> ...\n"+
			"Runs tests matched by zero or more patterns.\n\n", filepath.Base(os.Args[0]))
		flags.PrintDefaults()
	}

	cfg := runConfig{}
	list := flags.Bool("list", false, "print matched tests as JSON and exit")
	report := flags.Bool("report", false, "print output as control messages")
	flags.StringVar(&cfg.dataDir, "datadir", defaultDataDir,
		"directory where data files are located")
	flags.StringVar(&cfg.outDir, "outdir", "/tmp/tast/out",
		"base directory where tests write output files")

	var err error
	if err = flags.Parse(args); err != nil {
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

	if cfg.tests, err = testsToRun(flags.Args()); err != nil {
		writeError(fmt.Sprintf("Failed getting tests for %v: %v", flags.Args(), err.Error()))
		return nil, statusBadPatterns
	}
	sort.Slice(cfg.tests, func(i, j int) bool { return cfg.tests[i].Name < cfg.tests[j].Name })

	if *list {
		if err = testing.WriteTestsAsJSON(stdout, cfg.tests); err != nil {
			writeError(err.Error())
			return nil, statusError
		}
		return nil, statusSuccess
	}
	if *report {
		cfg.mw = control.NewMessageWriter(stdout)
	}
	return &cfg, statusSuccess
}

// testsToRun returns tests to run for a command invoked with args.
// If no arguments are supplied, all registered tests are returned.
// If a single argument is supplied and it is surrounded by parentheses,
// it is treated as a boolean expression specifying test attributes.
// Otherwise, argument(s) are interpreted as wildcard patterns matching test names.
func testsToRun(args []string) ([]*testing.Test, error) {
	if len(args) == 0 {
		return testing.GlobalRegistry().AllTests(), nil
	}
	if len(args) == 1 && strings.HasPrefix(args[0], "(") && strings.HasSuffix(args[0], ")") {
		return testing.GlobalRegistry().TestsForAttrExpr(args[0][1 : len(args[0])-1])
	}
	// Print a helpful error message if it looks like the user wanted an attribute expression.
	if len(args) == 1 && (strings.Contains(args[0], "&&") || strings.Contains(args[0], "||")) {
		return nil, fmt.Errorf("attr expr %q must be within parentheses", args[0])
	}
	return testing.GlobalRegistry().TestsForPatterns(args)
}

// runConfig describes how runTests should run tests.
type runConfig struct {
	// mw is used to send control messages to the controlling process.
	// It is initialized by parseArgs and is nil if the -report flag was not passed.
	mw *control.MessageWriter
	// outDir contains the base directory under which test output will be written.
	// It is initialized by parseArgs.
	outDir string
	// dataDir contains the base directory under which test data files are located.
	// It is initialized by parseArgs.
	dataDir string
	// tests contains tests to run. It is initialized by parseArgs.
	tests []*testing.Test

	// setupFunc is run before each test if non-nil.
	setupFunc func() error
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

// runTests runs tests per cfg.

// If an error is encountered in the test harness (as opposed to in a test), it is returned
// immediately.
//
// If cfg.mw is nil (i.e. tests were executed manually rather than by the tast command),
// failure is reported if any tests failed. If cfg.mw is non-nil, success is reported even
// if tests fail, as the tast command knows how to interpret test results.
func runTests(ctx context.Context, cfg *runConfig) int {
	if len(cfg.tests) == 0 {
		writeError("No tests matched by pattern(s)")
		return statusNoTests
	}

	numFailed := 0
	for _, t := range cfg.tests {
		// Make a copy of the test with the default timeout if none was specified.
		test := *t
		if test.Timeout == 0 {
			test.Timeout = cfg.defaultTestTimeout
		}

		if cfg.mw != nil {
			cfg.mw.WriteMessage(&control.TestStart{Time: time.Now(), Test: test})
		} else {
			logger.Print("Running ", test.Name)
		}

		outDir := filepath.Join(cfg.outDir, test.Name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			writeError("Failed to create output dir: " + err.Error())
			return statusError
		}

		if cfg.setupFunc != nil {
			if err := cfg.setupFunc(); err != nil {
				writeError("Failed to run setup: " + err.Error())
				return statusError
			}
		}
		ch := make(chan testing.Output)
		s := testing.NewState(ctx, ch, filepath.Join(cfg.dataDir, test.DataDir()), outDir,
			test.Timeout, test.CleanupTimeout)

		done := make(chan bool, 1)
		go func() {
			if succeeded := copyTestOutput(ch, cfg.mw); !succeeded {
				numFailed++
			}
			done <- true
		}()
		test.Run(s)
		close(ch)
		<-done

		if cfg.mw != nil {
			cfg.mw.WriteMessage(&control.TestEnd{Time: time.Now(), Name: test.Name})
		} else {
			logger.Printf("Finished %s", test.Name)
		}
	}

	if numFailed > 0 && cfg.mw == nil {
		writeError(fmt.Sprintf("%d test(s) failed", numFailed))
		return statusTestsFailed
	}
	return statusSuccess
}

// indent indents each line of s using prefix.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// copyTestOutput reads test output from ch and writes it to mw.
// If mw is nil, the output is just logged to stdout.
// true is returned if the test suceeded.
func copyTestOutput(ch chan testing.Output, mw *control.MessageWriter) (succeeded bool) {
	succeeded = true

	for o := range ch {
		if o.Err != nil {
			succeeded = false
			if mw != nil {
				mw.WriteMessage(&control.TestError{Time: o.T, Error: *o.Err})
			} else {
				stack := indent(strings.TrimSpace(o.Err.Stack), strings.Repeat(" ", logPrefixLen))
				logger.Printf("Error: [%s:%d] %v\n%s", o.Err.File, o.Err.Line, o.Err.Reason, stack)
			}
		} else {
			if mw != nil {
				mw.WriteMessage(&control.TestLog{Time: o.T, Text: o.Msg})
			} else {
				logger.Print(o.Msg)
			}
		}
	}

	return succeeded
}
