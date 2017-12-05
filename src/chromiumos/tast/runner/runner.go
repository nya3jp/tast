// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runner provides functionality shared by test executables.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

const (
	// Number of characters in prefixes from the log package, e.g. "2017/08/17 09:29:54 ".
	logPrefixLen = 20
)

// TestsToRun returns tests to run for a command invoked with args.
//
// If no arguments are supplied, all registered tests are returned.
// If a single argument is supplied and it is surrounded by parentheses,
// it is treated as a boolean expression specifying test attributes.
// Otherwise, argument(s) are interpreted as wildcard patterns matching test names.
//
// If any error was encountered while registering tests, it is returned instead.
func TestsToRun(args []string) ([]*testing.Test, error) {
	if errs := testing.RegistrationErrors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		return nil, errors.New(strings.Join(es, "\n"))
	}

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

// Log writes a RunLog control message to mw (if non-nil) or stdout.
func Log(mw *control.MessageWriter, msg string) {
	if mw != nil {
		mw.WriteMessage(&control.RunLog{time.Now(), msg})
	} else {
		log.Print(msg)
	}
}

// Abort writes a RunError control message to mw (if non-nil) and logs
// a fatal error.
func Abort(mw *control.MessageWriter, msg string) {
	if mw != nil {
		_, fn, ln, _ := runtime.Caller(1)
		mw.WriteMessage(&control.RunError{time.Now(), testing.Error{
			Reason: msg,
			File:   fn,
			Line:   ln,
			Stack:  string(debug.Stack()),
		}})
	}
	log.Fatal(msg)
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
// If mw is nil, the output is just logged to os.Stdout.
// true is returned if the test suceeded.
func copyTestOutput(ch chan testing.Output, mw *control.MessageWriter) (succeeded bool) {
	succeeded = true

	for o := range ch {
		if o.Err != nil {
			succeeded = false
			if mw != nil {
				mw.WriteMessage(&control.TestError{o.T, *o.Err})
			} else {
				stack := indent(strings.TrimSpace(o.Err.Stack), strings.Repeat(" ", logPrefixLen))
				log.Printf("Error: [%s:%d] %v\n%s", o.Err.File, o.Err.Line, o.Err.Reason, stack)
			}
		} else {
			if mw != nil {
				mw.WriteMessage(&control.TestLog{o.T, o.Msg})
			} else {
				log.Print(o.Msg)
			}
		}
	}

	return succeeded
}

// RunConfig contains a test-running configuration to be passed to RunTests.
type RunConfig struct {
	// Ctx is the context to be passed to tests.
	Ctx context.Context
	// Tests contains tests to run.
	Tests []*testing.Test
	// MessageWriter is used to send messages to the controlling process.
	// If nil, output is logged to stdout instead.
	MessageWriter *control.MessageWriter
	// SetupFunc is run before every test if non-nil.
	SetupFunc func() error
	// BaseOutDir contains the base directory under which test output will be written.
	BaseOutDir string
	// DataDir contains the base directory under which test data files are located.
	DataDir string
	// DefaultTestTimeout contains the default maximum time allotted to each test.
	// This is only used if testing.Test.Timeout is unset.
	DefaultTestTimeout time.Duration
}

// RunTests runs tests as dictated by cfg. The number of failing tests is returned.
// If an error is encountered in the test harness (as opposed to in a test),
// it is returned immediately.
func RunTests(cfg RunConfig) (numFailed int, err error) {
	for _, t := range cfg.Tests {
		// Make a copy of the test with the default timeout if none was specified.
		test := *t
		if test.Timeout == 0 {
			test.Timeout = cfg.DefaultTestTimeout
		}

		if cfg.MessageWriter != nil {
			cfg.MessageWriter.WriteMessage(&control.TestStart{time.Now(), test.Name, test})
		} else {
			log.Print("Running ", test.Name)
		}

		outDir := filepath.Join(cfg.BaseOutDir, test.Name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return 0, err
		}

		if cfg.SetupFunc != nil {
			cfg.SetupFunc()
		}
		ch := make(chan testing.Output)
		s := testing.NewState(cfg.Ctx, ch, filepath.Join(cfg.DataDir, test.DataDir()), outDir, test.Timeout)

		done := make(chan bool, 1)
		go func() {
			if succeeded := copyTestOutput(ch, cfg.MessageWriter); !succeeded {
				numFailed++
			}
			done <- true
		}()
		test.Run(s)
		close(ch)
		<-done

		if cfg.MessageWriter != nil {
			cfg.MessageWriter.WriteMessage(&control.TestEnd{time.Now(), test.Name})
		} else {
			log.Printf("Finished %s", test.Name)
		}
	}

	return numFailed, nil
}

// PrintTests marshals ts to JSON and writes the resulting data to w.
func PrintTests(w io.Writer, ts []*testing.Test) error {
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
