// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	gotesting "testing"

	"chromiumos/tast/bundle"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

const (
	// Prefix for bundles created by createBundleSymlinks.
	bundlePrefix = "fake_bundle"

	// Message written to stderr by runFakeBundle when no tests were registered.
	noTestsError = "no tests in bundle"
)

func init() {
	// If the binary was executed via a symlink created by createBundleSymlinks,
	// behave like a test bundle instead of running unit tests.
	if strings.HasPrefix(filepath.Base(os.Args[0]), bundlePrefix) {
		os.Exit(runFakeBundle())
	}
}

// createBundleSymlinks creates a temporary directory and places symlinks within it
// pointing at the test binary that's currently running. Each symlink is given a name
// that is later parsed by runFakeBundle to determine the bundle's desired behavior.
// The temporary directory's path is returned.
func createBundleSymlinks(t *gotesting.T, bundleTestResults ...[]bool) (dir string) {
	exec, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	dir = testutil.TempDir(t, "runner_test.")
	for bn, tests := range bundleTestResults {
		var s string
		for _, pass := range tests {
			if pass {
				s += "p"
			} else {
				s += "f"
			}
		}
		// Symlinks take the form "<prefix>-<bundleNum>-<testResults>", where bundleNum
		// is a 0-indexed integer and testResults is a string consisting of 'p' and 'f'
		// runes indicating whether each successive test should pass or fail.
		name := fmt.Sprintf("%s-%d-%s", bundlePrefix, bn, s)
		if err = os.Symlink(exec, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// getTestName returns the name to use for the test with the 0-indexed testNum in the
// bundle with the 0-indexed bundleNum.
func getTestName(bundleNum, testNum int) string {
	return fmt.Sprintf("pkg.Bundle%dTest%d", bundleNum, testNum)
}

// runFakeBundle is invoked when the test binary that's currently running is executed
// via a symlink previously created by createBundleSymlinks. The symlink name is parsed
// to get the desired behavior, and then tests are registered and executed via the
// bundle package's Local function.
func runFakeBundle() int {
	parts := strings.Split(filepath.Base(os.Args[0]), "-")
	if len(parts) != 3 {
		log.Fatalf("Unparsable filename %q", os.Args[0])
	}
	bundleNum, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Fatalf("Bad bundle number %q in filename %q", parts[1], os.Args[0])
	}

	// Write a hardcoded error message if no tests were specified.
	if len(parts[2]) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", noTestsError)
		os.Exit(1)
	}

	for i, res := range parts[2] {
		var f testing.TestFunc
		if res == 'p' {
			f = func(s *testing.State) {}
		} else if res == 'f' {
			f = func(s *testing.State) { s.Fatal("Failed") }
		} else {
			log.Fatalf("Bad rune %v in result string %q", res, parts[2])
		}
		testing.AddTest(&testing.Test{
			Name: getTestName(bundleNum, i),
			Func: f,
		})
	}

	return bundle.Local(os.Stdin, os.Stdout, os.Stderr)
}

// newBufferWithArgs returns a buffer containing the JSON representation of args.
func newBufferWithArgs(t *gotesting.T, args *Args) *bytes.Buffer {
	b := &bytes.Buffer{}
	if args != nil {
		if err := json.NewEncoder(b).Encode(args); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// callRun calls Run using the supplied arguments and returns its output, status, and a signature that can
// be used in error messages.
func callRun(t *gotesting.T, clArgs []string, stdinArgs, defaultArgs *Args, rt RunnerType) (
	status int, stdout, stderr *bytes.Buffer, sig string) {
	if defaultArgs == nil {
		defaultArgs = &Args{}
	}
	sig = fmt.Sprintf("Run(%v, %+v)", clArgs, stdinArgs)
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	return Run(clArgs, newBufferWithArgs(t, stdinArgs), stdout, stderr, defaultArgs, rt), stdout, stderr, sig
}

// readAllMessages reads and returns a slice of pointers to control messages
// from r. It reports a fatal test error if any problems are encountered.
func readAllMessages(t *gotesting.T, r io.Reader) []interface{} {
	msgs := make([]interface{}, 0)
	mr := control.NewMessageReader(r)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage failed: %v", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// gotRunError returns true if there is a control.RunError message in msgs,
// a slice of pointers to control messages.
func gotRunError(msgs []interface{}) bool {
	for _, msg := range msgs {
		if _, ok := msg.(*control.RunError); ok {
			return true
		}
	}
	return false
}

func TestRunListTests(t *gotesting.T) {
	// Create one bundle with two tests, one with three, and one with just one.
	dir := createBundleSymlinks(t, []bool{true, true}, []bool{true, true, true}, []bool{true})
	defer os.RemoveAll(dir)

	// All six tests should be printed.
	args := Args{
		Mode:       ListTestsMode,
		BundleGlob: filepath.Join(dir, "*"),
	}
	status, stdout, stderr, sig := callRun(t, nil, &args, nil, LocalRunner)
	if status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}

	var tests []*testing.Test
	if err := json.Unmarshal(stdout.Bytes(), &tests); err != nil {
		t.Fatalf("%s printed unparsable output %q", sig, stdout.String())
	}
	if len(tests) != 6 {
		t.Errorf("%s printed %v test(s); want 6: %v", sig, len(tests), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("%s wrote to stderr: %q", sig, stderr.String())
	}
}

func TestRunListTestsNoBundles(t *gotesting.T) {
	// Don't create any bundles; this should make the runner fail.
	dir := createBundleSymlinks(t)
	defer os.RemoveAll(dir)

	args := Args{
		Mode:       ListTestsMode,
		BundleGlob: filepath.Join(dir, "*"),
	}
	// The runner should only exit with 0 and report errors via control messages on stdout when it's
	// performing an actual test run. Since we're only listing tests, it should instead exit with an
	// error (and write the error to stderr, although that's not checked here).
	status, stdout, stderr, sig := callRun(t, nil, &args, nil, LocalRunner)
	if status != statusNoBundles {
		t.Errorf("%s = %v; want %v", sig, status, statusNoBundles)
	}
	if stdout.Len() != 0 {
		t.Errorf("%s wrote %q to stdout; want nothing (error should go to stderr)", sig, stdout.String())
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}
}

func TestRunSysInfo(t *gotesting.T) {
	td := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"logs/1.txt":    "first file",
		"crashes/1.dmp": "first crash",
	}); err != nil {
		t.Fatal(err)
	}

	// Get the initial state.
	defaultArgs := Args{
		SystemLogDir:    filepath.Join(td, "logs"),
		SystemCrashDirs: []string{filepath.Join(td, "crashes")},
	}
	status, stdout, _, sig := callRun(t, nil, &Args{Mode: GetSysInfoStateMode}, &defaultArgs, LocalRunner)
	if status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}
	var getRes GetSysInfoStateResult
	if err := json.NewDecoder(stdout).Decode(&getRes); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	if len(getRes.Warnings) > 0 {
		t.Errorf("%v produced warning(s): %v", sig, getRes.Warnings)
	}

	if err := testutil.WriteFiles(td, map[string]string{
		"logs/2.txt":    "second file",
		"crashes/2.dmp": "second crash",
	}); err != nil {
		t.Fatal(err)
	}

	// Now collect system info.
	args := Args{
		Mode:               CollectSysInfoMode,
		CollectSysInfoArgs: CollectSysInfoArgs{InitialState: getRes.State},
	}
	if status, stdout, _, sig = callRun(t, nil, &args, &defaultArgs, LocalRunner); status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}
	var collectRes CollectSysInfoResult
	if err := json.NewDecoder(stdout).Decode(&collectRes); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}
	if len(collectRes.Warnings) > 0 {
		t.Errorf("%v produced warning(s): %v", sig, collectRes.Warnings)
	}
	defer os.RemoveAll(collectRes.LogDir)
	defer os.RemoveAll(collectRes.CrashDir)

	// The newly-written files should have been copied into the returned temp dirs.
	if act, err := testutil.ReadFiles(collectRes.LogDir); err != nil {
		t.Error(err)
	} else {
		if exp := map[string]string{"2.txt": "second file"}; !reflect.DeepEqual(act, exp) {
			t.Errorf("%v collected logs %v; want %v", sig, act, exp)
		}
	}
	if act, err := testutil.ReadFiles(collectRes.CrashDir); err != nil {
		t.Error(err)
	} else {
		if exp := map[string]string{"2.dmp": "second crash"}; !reflect.DeepEqual(act, exp) {
			t.Errorf("%v collected crashes %v; want %v", sig, act, exp)
		}
	}
}

func TestRunTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{false, true}, []bool{true})
	defer os.RemoveAll(dir)

	// Run should execute multiple test bundles and merge their output correctly.
	status, stdout, stderr, sig := callRun(t, nil, &Args{BundleGlob: filepath.Join(dir, "*")}, nil, LocalRunner)
	if status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}

	msgs := readAllMessages(t, stdout)
	if rs, ok := msgs[0].(*control.RunStart); !ok {
		t.Errorf("First message not RunStart: %v", msgs[0])
	} else if rs.NumTests != 3 {
		t.Errorf("RunStart reported %v test(s); want 3", rs.NumTests)
	}
	if re, ok := msgs[len(msgs)-1].(*control.RunEnd); !ok {
		t.Errorf("Last message not RunEnd: %v", msgs[len(msgs)-1])
	} else {
		if _, err := os.Stat(re.OutDir); os.IsNotExist(err) {
			t.Errorf("RunEnd out dir %q doesn't exist", re.OutDir)
		} else {
			os.RemoveAll(re.OutDir)
		}
	}

	// Check that the right number of tests are reported as started and failed and that bundles and tests are sorted.
	// (Bundle names are included in these test names, so we can verify sorting by just comparing names.)
	tests := make(map[string]struct{})
	failed := make(map[string]struct{})
	var name string
	for _, msg := range msgs {
		if ts, ok := msg.(*control.TestStart); ok {
			if ts.Test.Name < name {
				t.Errorf("Saw unsorted test %q after %q", ts.Test.Name, name)
			}
			name = ts.Test.Name
			tests[name] = struct{}{}
		} else if _, ok := msg.(*control.TestError); ok {
			failed[name] = struct{}{}
		}
	}
	if len(tests) != 3 {
		t.Errorf("Got TestStart messages for %v test(s); want 3", len(tests))
	}
	if len(failed) != 1 {
		t.Errorf("Got TestError messages for %v test(s); want 1", len(failed))
	}

	if stderr.Len() != 0 {
		t.Errorf("%s wrote %q to stderr", sig, stderr.String())
	}
}

func TestInvalidFlag(t *gotesting.T) {
	status, stdout, stderr, sig := callRun(t, []string{"-bogus"}, nil, nil, LocalRunner)
	if status != statusBadArgs {
		t.Errorf("%s = %v; want %v", sig, status, statusBadArgs)
	}
	if stdout.Len() != 0 {
		t.Errorf("%s wrote %q to stdout; want nothing (error should go to stderr)", sig, stdout.String())
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}
}

func TestRunNoTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// RunTests should fail when run manually with a pattern is passed that doesn't match any tests.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), "bogus.SomeTest"}
	status, stdout, stderr, sig := callRun(t, clArgs, nil, nil, LocalRunner)
	if status != statusNoTests {
		t.Errorf("%s = %v; want %v", sig, status, statusNoTests)
	}
	if stdout.Len() != 0 {
		t.Errorf("%s wrote %q to stdout; want nothing (error should go to stderr)", sig, stdout.String())
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}

	// If the command was run by the tast command, it should exit with success.
	args := &Args{BundleGlob: filepath.Join(dir, "*"), Patterns: []string{"bogus.SomeTest"}}
	if status, stdout, stderr, sig = callRun(t, nil, args, nil, LocalRunner); status != statusSuccess {
		t.Errorf("%s = %v; want %v", sig, status, statusSuccess)
	}
	if gotRunError(readAllMessages(t, stdout)) {
		t.Errorf("%s wrote RunError message", sig)
	}
	if stderr.Len() != 0 {
		t.Errorf("%s wrote %q to stderr", sig, stderr.String())
	}
}

func TestRunFailForTestErrorWhenRunManually(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true, false, true})
	defer os.RemoveAll(dir)

	// RunTests should fail when a bundle reports failure while run manually.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*")}
	status, _, stderr, sig := callRun(t, clArgs, nil, nil, LocalRunner)
	if status != statusTestFailed {
		t.Errorf("%s = %v; want %v", sig, status, statusTestFailed)
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}
}

func TestRunPrintBundleError(t *gotesting.T) {
	// Without any tests, the bundle should report failure.
	dir := createBundleSymlinks(t, []bool{})
	defer os.RemoveAll(dir)

	// parseArgs should report success, but it should write a RunError control message.
	args := Args{
		Mode:       RunTestsMode,
		BundleGlob: filepath.Join(dir, "*"),
	}
	status, stdout, _, sig := callRun(t, nil, &args, nil, LocalRunner)
	if status != statusSuccess {
		t.Errorf("%s = %v; want %v", sig, status, statusSuccess)
	}

	// The RunError control message should contain the error message that the bundle wrote to stderr.
	var msg *control.RunError
	for _, m := range readAllMessages(t, stdout) {
		if re, ok := m.(*control.RunError); ok {
			msg = re
			break
		}
	}
	if msg == nil {
		t.Fatal("No RunError message for failed bundle")
	} else if !strings.Contains(msg.Error.Reason, noTestsError) {
		t.Fatalf("RunError message %q doesn't contain bundle stderr message %q", msg.Error.Reason, noTestsError)
	}
}

func TestSkipBundlesWithoutMatchedTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true}, []bool{true})
	defer os.RemoveAll(dir)

	// Bundles report failure if instructed to run using a pattern that doesn't
	// match any tests in the bundle. Pass the name of the test in the first bundle and
	// check that the run succeeds (i.e. the second bundle wasn't executed).
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), getTestName(0, 0)}
	status, _, _, sig := callRun(t, clArgs, nil, nil, LocalRunner)
	if status != statusSuccess {
		t.Errorf("%s = %v; want %v", sig, status, statusSuccess)
	}
}

func TestRunTestsUseRequestedOutDir(t *gotesting.T) {
	bundleDir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(bundleDir)
	outDir := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(outDir)

	status, stdout, _, sig := callRun(t, nil, &Args{BundleGlob: filepath.Join(bundleDir, "*"), OutDir: outDir}, nil, LocalRunner)
	if status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}

	msgs := readAllMessages(t, stdout)
	if re, ok := msgs[len(msgs)-1].(*control.RunEnd); !ok {
		t.Errorf("Last message not RunEnd: %v", msgs[len(msgs)-1])
	} else {
		// The RunEnd message should contain the out dir that we originally requested.
		if re.OutDir != outDir {
			t.Errorf("RunEnd.OutDir = %q; want %q", re.OutDir, outDir)
		}
	}
}
