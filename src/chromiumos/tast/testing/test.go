// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides infrastructure used by tests.
package testing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

const (
	testCleanupTimeout = 3 * time.Second // time for test cleanup after timeout

	testDataSubdir = "data" // subdir relative to test package containing data files

	testNameAttrPrefix   = "name:"   // prefix for auto-added attribute containing test name
	testBundleAttrPrefix = "bundle:" // prefix for auto-added attribute containing bundle name
)

// TestFunc is the code associated with a test.
type TestFunc func(*State)

// Test contains information about a test and its code itself.
//
// While this struct can be marshaled to a JSON object, note that unmarshaling that object
// will not yield a runnable Test struct; Func will not be present.
type Test struct {
	// Name specifies the test's name as "category.TestName". If empty (which it typically should be),
	// generated from Func's package and function name.
	Name string `json:"name"`
	// Func is the function to be executed to perform the test.
	Func TestFunc `json:"-"`
	// Desc is a short one-line description of the test.
	Desc string `json:"desc"`
	// Attr contains freeform text attributes describing the test.
	// See https://chromium.googlesource.com/chromiumos/platform/tast/+/master/docs/test_attributes.md
	// for commonly-used attributes.
	Attr []string `json:"attr"`
	// Data contains paths of data files needed by the test, relative to a "data" subdirectory within the
	// directory in which TestFunc is located.
	Data []string `json:"data"`
	// Timeout contains the maximum duration for which Func may run before the test is aborted.
	// This should almost always be omitted when defining tests; a reasonable default will be used.
	Timeout time.Duration `json:"timeout"`
	// Go package in which Func is located. This is filled automatically and should be omitted when
	// defining tests; it is only public so it can be included when this struct is marshaled.
	Pkg string `json:"pkg"`
}

// DataDir returns the path to the directory in which files listed in Data will be located,
// relative to the top-level directory containing data files.
func (tst *Test) DataDir() string {
	return filepath.Join(tst.Pkg, testDataSubdir)
}

// Run runs the test, passing s to it. The output channel is not closed automatically.
func (tst *Test) Run(s *State) {
	defer s.cancel()

	// Tests call runtime.Goexit() to make the current goroutine exit immediately
	// (after running defer blocks) on failure.
	done := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.Errorf("Panic: %v", r)
			}
			done <- true
		}()
		tst.Func(s)
	}()

	select {
	case <-done:
		// The goroutine running the test finished.
	case <-s.ctx.Done():
		s.Errorf("Test timed out: %v", s.ctx.Err())
		// If the test is using the context correctly, it should also give up soon.
		// Give it a bit of time to clean up before we move on.
		select {
		case <-done:
			// The test also noticed the timeout and finished cleaning up.
		case <-time.After(testCleanupTimeout):
			s.Logf("Test cleanup deadline exceeded")
			// TODO(derat): Might need to make sure it's dead somehow...
		}
	}
}

func (tst *Test) String() string {
	return tst.Name
}

// populateNameAndPkg fills Name (if empty) and Pkg (unconditionally).
func (tst *Test) populateNameAndPkg() error {
	if tst.Func == nil {
		return errors.New("missing function")
	}
	pkg, name, err := getTestFunctionPackageAndName(tst.Func)
	if err != nil {
		return err
	}

	if tst.Name == "" {
		p := strings.Split(pkg, "/")
		if len(p) < 2 {
			return fmt.Errorf("failed to split package %q into at least two components", pkg)
		}
		tst.Name = fmt.Sprintf("%s.%s", p[len(p)-1], name)
	}

	tst.Pkg = pkg

	return nil
}

// addAutoAttributes adds automatically-generated attributes to Attr.
// populateNameAndPkg must be called first.
func (tst *Test) addAutoAttributes() error {
	if tst.Name == "" {
		return errors.New("test name is empty")
	}
	if tst.Pkg == "" {
		return errors.New("test package is empty")
	}

	for _, attr := range tst.Attr {
		if strings.HasPrefix(attr, testNameAttrPrefix) || strings.HasPrefix(attr, testBundleAttrPrefix) {
			return fmt.Errorf("attribute %q has reserved prefix", attr)
		}
	}

	tst.Attr = append(tst.Attr, testNameAttrPrefix+tst.Name)
	if comps := strings.Split(tst.Pkg, "/"); len(comps) >= 2 {
		tst.Attr = append(tst.Attr, testBundleAttrPrefix+comps[len(comps)-2])
	}
	return nil
}

// getTestFunctionPackageAndName determines the package and name for f.
func getTestFunctionPackageAndName(f TestFunc) (pkg, name string, err error) {
	rf := runtime.FuncForPC(reflect.ValueOf(f).Pointer())
	if rf == nil {
		return "", "", errors.New("failed to get function from PC")
	}
	p := strings.SplitN(rf.Name(), ".", 2)
	if len(p) != 2 {
		return "", "", fmt.Errorf("didn't find package.function in %q", rf.Name())
	}
	return p[0], p[1], nil
}

// WriteTestsAsJSON marshals ts to JSON and writes the resulting data to w.
func WriteTestsAsJSON(w io.Writer, ts []*Test) error {
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
