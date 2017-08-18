// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides infrastructure used by tests.
package testing

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

const (
	testCleanupTimeout = 3 * time.Second
	testDataSubdir     = "data"
)

// TestFunc is the code associated with a test.
type TestFunc func(*State)

// Test contains information about a test and its code itself.
type Test struct {
	// Test name. If empty, generated from Func's package and function name.
	Name string
	// The test function.
	Func TestFunc
	// Short one-line description of the test.
	Desc string
	// Attributes describing the test.
	Attr []string
	// Paths of data files needed by the test, relative to a "data" subdirectory within the
	// directory in which TestFunc is located. Any occurrences of the substring "{arch}"
	// will be replaced by the DUT's architecture, e.g. "i686", "x86_64", "armv7l", or "aarch64".
	Data []string

	// Package in which Func is located.
	pkg string
}

// DataDir returns the path to the directory in which files listed in Data will be located,
// relative to the top-level directory containing data files.
func (tst *Test) DataDir() string {
	return filepath.Join(tst.pkg, testDataSubdir)
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

// TestDataPathForArch replaces occurrences of "{arch}" with arch in p,
// a path from Test.Data.
func TestDataPathForArch(p, arch string) string {
	return strings.Replace(p, "{arch}", arch, -1)
}

// populateNameAndPkg fills Name (if empty) and pkg (unconditionally).
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

	tst.pkg = pkg

	return nil
}

// getTestFunctionPackageAndName determines the package and name for f.
func getTestFunctionPackageAndName(f TestFunc) (pkg, name string, err error) {
	rf := runtime.FuncForPC(reflect.ValueOf(f).Pointer())
	if rf == nil {
		return "", "", errors.New("failed to get function from PC")
	}
	p := strings.Split(rf.Name(), ".")
	if len(p) != 2 {
		return "", "", fmt.Errorf("didn't find package.function in %q", rf.Name())
	}
	return p[0], p[1], nil
}
