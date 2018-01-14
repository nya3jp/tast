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
	"log"
	"os"
	"path/filepath"
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
)

func init() {
	// If the binary was executed via a symlink created by createBundleSymlinks,
	// behave like a test bundle instead of running unit tests.
	if strings.HasPrefix(filepath.Base(os.Args[0]), bundlePrefix) {
		os.Exit(runBundle())
	}
}

// createBundleSymlinks creates a temporary directory and places symlinks within it
// pointing at the test binary that's currently running. Each symlink is given a name
// that is later parsed by runBundle to determine the bundle's desired behavior.
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

// runBundle is invoked when the test binary that's currently running is executed
// via a symlink previously created by createBundleSymlinks. The symlink name is parsed
// to get the desired behavior, and then tests are registered and executed via the
// bundle package's Local function.
func runBundle() int {
	parts := strings.Split(filepath.Base(os.Args[0]), "-")
	if len(parts) != 3 {
		log.Fatalf("Unparsable filename %q", os.Args[0])
	}
	bundleNum, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Fatalf("Bad bundle number %q in filename %q", parts[1], os.Args[0])
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

	return bundle.Local(os.Args[1:])
}

// callParseArgs calls ParseArgs with the supplied parameters.
// It reports a fatal test error if the returned status doesn't match expStatus or
// the returned RunConfig is nil when wantCfg is true or vice versa.
// The returned sig string can be used in test errors to describe the ParseArgs call.
func callParseArgs(t *gotesting.T, w io.Writer, args []string, glob string, flags *flag.FlagSet,
	expStatus int, wantCfg bool) (cfg *RunConfig, sig string) {
	cfg, status := ParseArgs(w, args, glob, "", flags)
	sig = fmt.Sprintf("ParseArgs(w, %v, %q, ...)", args, glob)
	if status != expStatus {
		t.Fatalf("%s returned status %v; want %v", sig, status, expStatus)
	} else if cfg == nil && wantCfg {
		t.Fatalf("%s returned nil RunConfig", sig)
	} else if cfg != nil && !wantCfg {
		t.Fatalf("%s returned non-nil RunConfig %v", sig, cfg)
	}
	return cfg, sig
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

func TestParseArgsListTests(t *gotesting.T) {
	// Create one bundle with two tests, one with three, and one with just one.
	dir := createBundleSymlinks(t, []bool{true, true}, []bool{true, true, true}, []bool{true})
	defer os.RemoveAll(dir)

	// All six tests should be printed.
	b := bytes.Buffer{}
	_, sig := callParseArgs(t, &b, []string{"-listtests"}, filepath.Join(dir, "*"), nil, statusSuccess, false)
	var tests []*testing.Test
	if err := json.Unmarshal(b.Bytes(), &tests); err != nil {
		t.Fatalf("%s printed unparsable output %q", sig, b.String())
	}
	if len(tests) != 6 {
		t.Errorf("%s printed %v test(s); want 6: %v", sig, len(tests), b.String())
	}
}

func TestRunTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{false, true}, []bool{true})
	defer os.RemoveAll(dir)

	// RunTests should execute multiple test bundles and merge their output correctly.
	b := bytes.Buffer{}
	cfg, _ := callParseArgs(t, &b, []string{"-report"}, filepath.Join(dir, "*"), nil, statusSuccess, true)

	// Check that pre-run and post-run functions are called.
	preRunCalled := false
	cfg.PreRun = func(mw *control.MessageWriter) { preRunCalled = true }
	const crashDir = "/tmp/crashes"
	cfg.PostRun = func(mw *control.MessageWriter) control.RunEnd {
		return control.RunEnd{CrashDir: crashDir}
	}

	if status := RunTests(cfg); status != statusSuccess {
		t.Fatalf("RunConfig(%v) = %v; want %v", cfg, status, statusSuccess)
	}

	msgs := readAllMessages(t, &b)
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
		if re.CrashDir != crashDir {
			t.Errorf("RunEnd contains crash dir %q; want %q", re.CrashDir, crashDir)
		}
	}
	if !preRunCalled {
		t.Error("Pre-run function not called")
	}

	// Check that the right number of tests are reported as started and failed.
	tests := make(map[string]struct{})
	failed := make(map[string]struct{})
	var name string
	for _, msg := range msgs {
		if ts, ok := msg.(*control.TestStart); ok {
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
}

func TestParseArgsInvalidFlag(t *gotesting.T) {
	// ParseArgs should fail when an invalid flag is passed.
	callParseArgs(t, &bytes.Buffer{}, []string{"-bogus"}, "/bogus/*", nil, statusBadArgs, false)

	// This should also happen when -report is passed.
	callParseArgs(t, &bytes.Buffer{}, []string{"-report", "-bogus"}, "/bogus/*", nil, statusBadArgs, false)
}

func TestParseArgsCustomFlag(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// When a flag.FlagSet is passed to ParseArgs, it should be used.
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	custom := flags.Bool("custom", false, "custom flag")
	_, sig := callParseArgs(t, &bytes.Buffer{}, []string{"-custom", "-report"},
		filepath.Join(dir, "*"), flags, statusSuccess, true)
	if !*custom {
		t.Errorf("%s didn't set -custom flag", sig)
	}
}

func TestParseArgsNoBundles(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// ParseArgs should fail when a glob is passed that doesn't match any bundles.
	callParseArgs(t, &bytes.Buffer{}, []string{}, filepath.Join(dir, "bogus*"), nil,
		statusNoBundles, false)

	// When -report is passed, the command should exit with success but write a
	// RunError control message.
	b := bytes.Buffer{}
	_, sig := callParseArgs(t, &b, []string{"-report"}, filepath.Join(dir, "bogus*"), nil,
		statusSuccess, false)
	if !gotRunError(readAllMessages(t, &b)) {
		t.Fatalf("%s didn't write RunError message", sig)
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// RunTests should fail when a pattern is passed that doesn't match any tests and -report isn't passed.
	cfg, _ := callParseArgs(t, &bytes.Buffer{}, []string{"bogus.SomeTest"}, filepath.Join(dir, "*"), nil,
		statusSuccess, true)
	if status := RunTests(cfg); status != statusNoTests {
		t.Fatalf("RunTests(%v) = %v; want %v", cfg, status, statusNoTests)
	}

	// If -report is passed, the command should exit with success.
	b := bytes.Buffer{}
	cfg, _ = callParseArgs(t, &b, []string{"-report", "bogus.SomeTest"}, filepath.Join(dir, "*"), nil,
		statusSuccess, true)
	if status := RunTests(cfg); status != statusSuccess {
		t.Fatalf("RunTests(%v) = %v; want %v", cfg, status, statusSuccess)
	}
	if gotRunError(readAllMessages(t, &b)) {
		t.Fatalf("RunTests(%v) wrote RunError message", cfg)
	}
}

func TestRunTestsFailForErrorWhenRunManually(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true, false, true})
	defer os.RemoveAll(dir)

	// RunTests should fail when a bundle reports failure and -report wasn't passed.
	cfg, _ := callParseArgs(t, &bytes.Buffer{}, []string{}, filepath.Join(dir, "*"), nil,
		statusSuccess, true)
	if status := RunTests(cfg); status != statusBundleFailed {
		t.Fatalf("RunConfig(%v) = %v; want %v", cfg, status, statusBundleFailed)
	}
}

func TestSkipBundlesWithoutMatchedTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true}, []bool{true})
	defer os.RemoveAll(dir)

	// Test bundles report failure if instructed to run using a pattern that doesn't
	// match any tests in the bundle. Pass the name of the test in the first bundle and
	// check that the run succeeds (i.e. the second bundle wasn't executed).
	cfg, _ := callParseArgs(t, &bytes.Buffer{}, []string{getTestName(0, 0)},
		filepath.Join(dir, "*"), nil, statusSuccess, true)
	if status := RunTests(cfg); status != statusSuccess {
		t.Fatalf("RunConfig(%v) = %v; want %v", cfg, status, statusSuccess)
	}
}
