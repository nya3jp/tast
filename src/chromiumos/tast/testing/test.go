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
	"sort"
	"strings"
	"time"
)

const (
	testDataSubdir = "data" // subdir relative to test package containing data files

	testNameAttrPrefix   = "name:"   // prefix for auto-added attribute containing test name
	testBundleAttrPrefix = "bundle:" // prefix for auto-added attribute containing bundle name
	testDepAttrPrefix    = "dep:"    // prefix for auto-added attribute containing software dependency
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
	// SoftwareDeps lists software features that are required to run the test.
	// If any dependencies are not satisfied by the DUT, the test will be skipped.
	// See https://chromium.googlesource.com/chromiumos/platform/tast/+/master/docs/test_dependencies.md
	// for more information about dependencies.
	SoftwareDeps []string `json:"softwareDeps,omitempty"`
	// Timeout contains the maximum duration for which Func may run before the test is aborted.
	// This should almost always be omitted when defining tests; a reasonable default will be used.
	// This field is serialized as an integer nanosecond count.
	Timeout time.Duration `json:"timeout"`
	// CleanupTimeout contains the maximum duration to wait for the test to clean up after a timeout.
	// This is exposed for unit tests and should almost always be omitted when defining tests;
	// a reasonable default will be used.
	CleanupTimeout time.Duration `json:"-"`
	// Pkg contains the Go package in which Func is located. This is filled automatically and should be
	// omitted when defining tests; it is only public so it can be included when this struct is marshaled.
	Pkg string `json:"pkg"`
}

// DataDir returns the path to the directory in which files listed in Data will be located,
// relative to the top-level directory containing data files.
func (tst *Test) DataDir() string {
	return filepath.Join(tst.Pkg, testDataSubdir)
}

// Run runs the test, passing s to it, and blocks until the test has either finished or the deadline
// (tst.Timeout plus tst.CleanupTimeout) is reached, whichever comes first.
//
// The test function executes in a goroutine and may still be running if it ignores its deadline;
// the returned value indicates whether the test completed within the allotted time or not.
// The output channel associated with s is only closed after the test function completes, so
// if false is returned, the caller is responsible for reporting that the test timed out.
func (tst *Test) Run(s *State) bool {
	defer func() {
		s.tcancel()
		s.cancel()
	}()

	// Tests call runtime.Goexit() to make the current goroutine exit immediately
	// (after running defer blocks) on failure.
	done := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.Errorf("Panic: %v", r)
			}
			close(s.ch)
			done <- true
		}()
		tst.Func(s)
	}()

	select {
	case <-done:
		return true
	case <-s.ctx.Done():
		// TODO(derat): Do more to try to kill the runaway test function.
		return false
	}
}

func (tst *Test) String() string {
	return tst.Name
}

// MissingSoftwareDeps returns a sorted list of dependencies from SoftwareDeps
// that aren't present on the DUT (per the passed-in features list).
func (tst *Test) MissingSoftwareDeps(features []string) []string {
	var missing []string
DepLoop:
	for _, d := range tst.SoftwareDeps {
		for _, f := range features {
			if d == f {
				continue DepLoop
			}
		}
		missing = append(missing, d)
	}
	sort.Strings(missing)
	return missing
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

// validateDataPath validates data paths.
func (tst *Test) validateDataPath() error {
	for _, p := range tst.Data {
		if p != filepath.Clean(p) || strings.HasPrefix(p, ".") || strings.HasPrefix(p, "/") {
			return fmt.Errorf("data path %q is invalid", p)
		}
	}
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
		for _, pre := range []string{testNameAttrPrefix, testBundleAttrPrefix, testDepAttrPrefix} {
			if strings.HasPrefix(attr, pre) {
				return fmt.Errorf("attribute %q has reserved prefix", attr)
			}
		}
	}

	tst.Attr = append(tst.Attr, testNameAttrPrefix+tst.Name)
	if comps := strings.Split(tst.Pkg, "/"); len(comps) >= 2 {
		tst.Attr = append(tst.Attr, testBundleAttrPrefix+comps[len(comps)-2])
	}
	for _, dep := range tst.SoftwareDeps {
		tst.Attr = append(tst.Attr, testDepAttrPrefix+dep)
	}
	return nil
}

// clone returns a deep copy of t.
func (t *Test) clone() *Test {
	copyable := func(tp reflect.Type) bool {
		// If copyable structs are added, they can be handled in a reflect.Struct case.
		switch tp.Kind() {
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint,
			reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64,
			reflect.Func, reflect.String:
			return true
		default:
			return false
		}
	}

	ov := reflect.ValueOf(*t)
	np := reflect.New(ov.Type()) // *Test
	nv := reflect.Indirect(np)   // Test

	for i := 0; i < ov.NumField(); i++ {
		of, nf := ov.Field(i), nv.Field(i)
		switch {
		case copyable(of.Type()):
			nf.Set(of)
		case of.Kind() == reflect.Slice && copyable(of.Type().Elem()):
			if !of.IsNil() {
				nf.Set(reflect.MakeSlice(of.Type(), of.Len(), of.Len()))
				reflect.Copy(nf, of)
			}
		default:
			panic(fmt.Sprintf("unable to copy Test.%s field of type %s", ov.Type().Field(i).Name, of.Type().Name()))
		}
	}

	return np.Interface().(*Test)
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
