// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runner provides functionality shared by test executables.
package runner

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"chromiumos/tast/common/control"
	"chromiumos/tast/common/testing"
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

// CopyTestOutput reads test output from ch and writes it to mw.
// If mw is nil, the output is just logged to os.Stdout.
// true is returned if the test suceeded.
func CopyTestOutput(ch chan testing.Output, mw *control.MessageWriter) (succeeded bool) {
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
