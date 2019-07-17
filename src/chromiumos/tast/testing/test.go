// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides infrastructure used by tests.
package testing

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	// ExternalLinkSuffix is a file name suffix for external data link files.
	// These are JSON files that can be unmarshaled into the externalLink struct.
	ExternalLinkSuffix = ".external"

	// ExternalErrorSuffix is a file name suffix for external data download error files.
	// An error message is written to the file when we encounter an error downloading
	// the corresponding external data file. This mechanism is used to pass errors from
	// the test runner (which downloads the files) to the test bundle so the bundle
	// can include them in the test's output.
	ExternalErrorSuffix = ".external-error"
)

// TestFunc is the code associated with a test.
type TestFunc func(context.Context, *State)

// Test contains information about a test and its code itself.
// This is designed to declare each test.
type Test struct {
	// Func is the function to be executed to perform the test.
	Func TestFunc

	// Desc is a short one-line description of the test.
	Desc string

	// Contacts is a list of email addresses of persons and groups who are familiar with the test.
	// At least one personal email address of an active committer should be specified so that we can
	// file bugs or ask for code reviews.
	Contacts []string

	// Attr contains freeform text attributes describing the test.
	// See https://chromium.googlesource.com/chromiumos/platform/tast/+/master/docs/test_attributes.md
	// for commonly-used attributes.
	Attr []string

	// Data contains paths of data files needed by the test, relative to a "data" subdirectory within the
	// directory in which Func is located.
	Data []string

	// Vars contains the names of runtime variables used to pass out-of-band data to tests.
	// Values are supplied using "tast run -var=name=value", and tests can access values via State.Var.
	Vars []string

	// SoftwareDeps lists software features that are required to run the test.
	// If any dependencies are not satisfied by the DUT, the test will be skipped.
	// See https://chromium.googlesource.com/chromiumos/platform/tast/+/master/docs/test_dependencies.md
	// for more information about dependencies.
	SoftwareDeps []string

	// Pre contains a precondition that must be met before the test is run.
	Pre Precondition

	// Timeout contains the maximum duration for which Func may run before the test is aborted.
	// This should almost always be omitted when defining tests; a reasonable default will be used.
	// This field is serialized as an integer nanosecond count.
	Timeout time.Duration
}

func validateTest(t *Test) error {
	info, err := getTestFuncInfo(t.Func)
	if err != nil {
		return err
	}

	if err := validateName(info.name, info.category, filepath.Base(info.file)); err != nil {
		return err
	}
	if err := validateAttr(t.Attr); err != nil {
		return err
	}
	if err := validateData(t.Data); err != nil {
		return err
	}
	if t.Timeout < 0 {
		return fmt.Errorf("%s.%s has negative timeout %v", info.category, info.name, t.Timeout)
	}
	if t.Pre != nil {
		if _, ok := t.Pre.(preconditionImpl); !ok {
			return fmt.Errorf("precondition %s does not implement preconditionImpl", t.Pre)
		}
	}

	return nil
}

// testFuncInfo contains information about a TestFunc.
type testFuncInfo struct {
	pkg      string // package name, e.g. "chromiumos/tast/local/bundles/cros/ui"
	category string // Tast category name, e.g. "ui". The last component of pkg
	name     string // function name, e.g. "ChromeLogin"
	file     string // full source path, e.g. "/home/user/chromeos/src/platform/tast-tests/.../ui/chrome_login.go"
}

// getTestFuncInfo returns info about f.
func getTestFuncInfo(f TestFunc) (*testFuncInfo, error) {
	if f == nil {
		return nil, errors.New("Func is nil")
	}
	pc := reflect.ValueOf(f).Pointer()
	rf := runtime.FuncForPC(pc)
	if rf == nil {
		return nil, errors.New("failed to get function from PC")
	}
	p := strings.SplitN(rf.Name(), ".", 2)
	if len(p) != 2 {
		return nil, fmt.Errorf("didn't find package.function in %q", rf.Name())
	}

	cs := strings.Split(p[0], "/")
	if len(cs) < 2 {
		return nil, fmt.Errorf("failed to split package %q into at least two components", p[0])
	}

	info := &testFuncInfo{
		pkg:      p[0],
		category: cs[len(cs)-1],
		name:     p[1],
	}
	info.file, _ = rf.FileLine(pc)
	return info, nil
}

// testNameRegexp validates test names, which should consist of a package name,
// a period, and the name of the exported test function.
var testNameRegexp = regexp.MustCompile("^[a-z][a-z0-9]*\\.[A-Z][A-Za-z0-9]*$")

// testWordRegexp validates an individual word in a test function name.
// See checkFuncNameAgainstFilename for details.
var testWordRegexp = regexp.MustCompile("^[A-Z0-9]+[a-z0-9]*[A-Z0-9]*$")

func validateName(funcName, category, filename string) error {
	name := fmt.Sprintf("%s.%s", category, funcName)
	if !testNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid test name %q (want pkg.ExportedTestFunc)", name)
	}

	if strings.ToLower(filename) != filename {
		return fmt.Errorf("filename %q isn't lowercase", filename)
	}
	const goExt = ".go"
	if filepath.Ext(filename) != goExt {
		return fmt.Errorf("filename %q doesn't have extension %q", filename, goExt)
	}

	// First, split the name into words based on underscores in the filename.
	funcIdx := 0
	fileWords := strings.Split(filename[:len(filename)-len(goExt)], "_")
	for _, fileWord := range fileWords {
		// Disallow repeated underscores.
		if len(fileWord) == 0 {
			return fmt.Errorf("empty word in filename %q", filename)
		}

		// Extract the characters from the function name corresponding to the word from the filename.
		if funcIdx+len(fileWord) > len(funcName) {
			return fmt.Errorf("name %q doesn't include all of filename %q", funcName, filename)
		}
		funcWord := funcName[funcIdx : funcIdx+len(fileWord)]
		if strings.ToLower(funcWord) != strings.ToLower(fileWord) {
			return fmt.Errorf("word %q at %q[%d] doesn't match %q in filename %q", funcWord, funcName, funcIdx, fileWord, filename)
		}

		// Test names are taken from Go function names, so they should follow Go's naming conventions.
		// Generally speaking, that means camel case with acronyms fully capitalized (although we can't catch
		// miscapitalized acronyms here, as we don't know if a given word is an acronym or not).
		// Every word should begin with either an uppercase letter or a digit.
		// Multiple leading or trailing uppercase letters are allowed to permit filename -> func-name pairings like
		// dbus.go -> "DBus", webrtc.go -> "WebRTC", and crosvm.go -> "CrosVM".
		// Note that this also permits incorrect filenames like loadurl.go for "LoadURL", but that's not something code can prevent.
		if !testWordRegexp.MatchString(funcWord) {
			return fmt.Errorf("word %q at %q[%d] should probably be %q (acronyms also allowed at beginning and end)",
				funcWord, funcName, funcIdx, strings.Title(strings.ToLower(funcWord)))
		}

		funcIdx += len(funcWord)
	}

	if funcIdx < len(funcName) {
		return fmt.Errorf("name %q has extra suffix %q not in filename %q", funcName, funcName[funcIdx:], filename)
	}

	return nil
}

func validateAttr(attr []string) error {
	for _, a := range attr {
		for _, pre := range []string{testNameAttrPrefix, testBundleAttrPrefix, testDepAttrPrefix} {
			if strings.HasPrefix(a, pre) {
				return fmt.Errorf("attribute %q has reserved prefix", a)
			}
		}
	}
	return nil
}

func validateData(data []string) error {
	for _, p := range data {
		if p != filepath.Clean(p) || strings.HasPrefix(p, ".") || strings.HasPrefix(p, "/") {
			return fmt.Errorf("data path %q is invalid", p)
		}
	}
	return nil
}
