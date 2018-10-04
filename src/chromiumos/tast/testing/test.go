// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides infrastructure used by tests.
package testing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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

	defaultTestCleanupTimeout = 3 * time.Second // extra time granted to tests to handle timeouts
)

var testNameRegexp, testWordRegexp *regexp.Regexp

func init() {
	// Validates test names, which should consist of a package name, a period,
	// and the name of the exported test function.
	testNameRegexp = regexp.MustCompile("^[a-z][a-z0-9]*\\.[A-Z][A-Za-z0-9]*$")

	// Validates an individual word in a test function name.
	// See checkFuncNameAgainstFilename for details.
	testWordRegexp = regexp.MustCompile("^[A-Z0-9]+[a-z0-9]*[A-Z0-9]*$")
}

// TestFunc is the code associated with a test.
type TestFunc func(context.Context, *State)

// Test contains information about a test and its code itself.
//
// While this struct can be marshaled to a JSON object, note that unmarshaling that object
// will not yield a runnable Test struct; Func will not be present.
type Test struct {
	// Name specifies the test's name as "category.TestName".
	// This is automatically derived from Func's package and function name and
	// must be left blank when registering a new test.
	// The category is the final component of the package.
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
	// directory in which Func is located.
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
	// The context passed to Func has a deadline based on Timeout, but Tast waits for CleanupTimeout to elapse
	// before reporting that the test has timed out; this gives the test function time to return after it
	// sees that its context has expired before an additional error is added about the timeout.
	// This is exposed for unit tests and should almost always be omitted when defining tests;
	// a reasonable default will be used.
	CleanupTimeout time.Duration `json:"-"`
	// Pkg contains the Go package in which Func is located.
	// Automatically filled using Func's package name.
	Pkg string `json:"pkg"`
}

// DataDir returns the path to the directory in which files listed in Data will be located,
// relative to the top-level directory containing data files.
func (tst *Test) DataDir() string {
	return filepath.Join(tst.Pkg, testDataSubdir)
}

// Run runs the test per cfg and blocks until the test has either finished or the deadline
// (tst.Timeout plus tst.CleanupTimeout) is reached, whichever comes first.
//
// The test function executes in a goroutine and may still be running if it ignores its deadline;
// the returned value indicates whether the test completed within the allotted time or not.
// ch is only closed after the test function completes, so if false is returned,
// the caller is responsible for reporting that the test timed out.
func (tst *Test) Run(ctx context.Context, ch chan Output, cfg *TestConfig) bool {
	// Attach the state to a context so support packages can log to it.
	s := newState(tst, ch, cfg)
	ctx = context.WithValue(ctx, logKey, s)

	cleanupTimeout := tst.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultTestCleanupTimeout
	}
	cleanupCtx, cleanupCancel := timeoutContext(ctx, extendTimeout(tst.Timeout, cleanupTimeout))
	defer cleanupCancel()
	testCtx, testCancel := timeoutContext(cleanupCtx, tst.Timeout)
	defer testCancel()

	// Tests call runtime.Goexit() to make the current goroutine exit immediately
	// (after running defer blocks) on failure.
	done := make(chan bool, 1)
	go func() {
		defer func() {
			close(s.ch)
			done <- true
		}()

		if cfg.OutDir != "" { // often left blank for unit tests
			if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
				s.Fatal("Failed to create output dir: ", err)
			}
		}

		if cfg.SetupFunc != nil {
			runAndRecover(cfg.SetupFunc, testCtx, s)
			if s.HasError() {
				// If the setup panicked or reported errors, do not run the test body nor the cleanup.
				return
			}
		}
		defer func() {
			if cfg.CleanupFunc != nil {
				runAndRecover(cfg.CleanupFunc, cleanupCtx, s)
			}
		}()

		runAndRecover(tst.Func, testCtx, s)
	}()

	select {
	case <-done:
		return true
	case <-cleanupCtx.Done():
		// TODO(derat): Do more to try to kill the runaway test function.
		return false
	}
}

// extendTimeout adds extra to timeout and returns the resulting duration.
// If timeout is zero or negative (indicating an unset timeout), zero is returned.
func extendTimeout(timeout, extra time.Duration) time.Duration {
	if timeout <= 0 {
		return 0
	}
	return timeout + extra
}

// timeoutContext returns a context and cancelation function derived from ctx with the specified timeout.
// If timeout is zero or negative (indicating an unset timeout), no timeout will be applied.
func timeoutContext(ctx context.Context, timeout time.Duration) (tctx context.Context, cancel func()) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// runAndRecover runs a test function with the given Context and State, and recovers if it panicked.
func runAndRecover(f func(context.Context, *State), ctx context.Context, s *State) {
	defer func() {
		if r := recover(); r != nil {
			s.Error("Panic: ", r)
		}
	}()
	f(ctx, s)
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

// finalize fills in defaults and validates the result.
// If autoName is true, tst.Name will be derived from tst.Func's name.
// Otherwise (just used in unit tests), tst.Name should be filled already.
func (tst *Test) finalize(autoName bool) error {
	// Fill in defaults.
	if err := tst.populateNameAndPkg(autoName); err != nil {
		return err
	}
	if err := tst.addAutoAttributes(); err != nil {
		return err
	}

	// Validate the result.
	if err := tst.validateTestName(); err != nil {
		return err
	}
	if err := tst.validateDataPath(); err != nil {
		return err
	}
	if tst.Timeout < 0 {
		return fmt.Errorf("%q has negative timeout %v", tst.Name, tst.Timeout)
	}
	return nil
}

// populateNameAndPkg fills Name and Pkg.
// If autoName is true, tst.Name will be derived from tst.Func's name.
// tst.Func's name will also be verified to match the name of the source file that declared it.
// Otherwise (just used in unit tests), tst.Name should be filled already.
func (tst *Test) populateNameAndPkg(autoName bool) error {
	if tst.Func == nil {
		return errors.New("missing function")
	}
	info, err := getTestFuncInfo(tst.Func)
	if err != nil {
		return err
	}

	p := strings.Split(info.pkg, "/")
	if len(p) < 2 {
		return fmt.Errorf("failed to split package %q into at least two components", info.pkg)
	}
	category := p[len(p)-1]

	if autoName {
		if tst.Name != "" {
			return fmt.Errorf("manually-assigned test name %q", tst.Name)
		}
		if err = checkFuncNameAgainstFilename(info.name, filepath.Base(info.file)); err != nil {
			return err
		}
		tst.Name = fmt.Sprintf("%s.%s", category, info.name)
	} else if tst.Name == "" {
		return fmt.Errorf("missing name for test with func %s", info.name)
	}

	tst.Pkg = info.pkg
	return nil
}

// validateTestName returns an error if the test name is invalid.
func (tst *Test) validateTestName() error {
	if !testNameRegexp.MatchString(tst.Name) {
		return fmt.Errorf("invalid test name %q (want pkg.ExportedTestFunc)", tst.Name)
	}
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

// testFuncInfo contains information about a TestFunc.
type testFuncInfo struct {
	pkg  string // package name, e.g. "chromiumos/tast/local/bundles/cros/ui"
	name string // function name, e.g. "ChromeLogin"
	file string // full source path, e.g. "/home/user/chromeos/src/platform/tast-tests/.../ui/chrome_login.go"
}

// getTestFuncInfo returns info about f.
func getTestFuncInfo(f TestFunc) (*testFuncInfo, error) {
	pc := reflect.ValueOf(f).Pointer()
	rf := runtime.FuncForPC(pc)
	if rf == nil {
		return nil, errors.New("failed to get function from PC")
	}
	p := strings.SplitN(rf.Name(), ".", 2)
	if len(p) != 2 {
		return nil, fmt.Errorf("didn't find package.function in %q", rf.Name())
	}

	info := &testFuncInfo{
		pkg:  p[0],
		name: p[1],
	}
	info.file, _ = rf.FileLine(pc)
	return info, nil
}

// checkFuncNameAgainstFilename verifies that a test function name (e.g. "MyTest") matches
// the name of the file that contains it (e.g. "my_test.go").
func checkFuncNameAgainstFilename(funcName, filename string) error {
	if strings.ToLower(filename) != filename {
		return fmt.Errorf("filename %q isn't lowercase", filename)
	}

	const goExt = ".go"
	ext := filepath.Ext(filename)
	if ext != goExt {
		return fmt.Errorf("filename %q doesn't have extension %q", filename, goExt)
	}

	// First, split the name into words based on underscores in the filename.
	funcIdx := 0
	fileWords := strings.Split(filename[:len(filename)-len(ext)], "_")
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

// WriteTestsAsJSON marshals ts to JSON and writes the resulting data to w.
func WriteTestsAsJSON(w io.Writer, ts []*Test) error {
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
