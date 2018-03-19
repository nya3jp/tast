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

// callParseArgsStdin calls ParseArgs with passedArgs JSON-marshaled to stdin.
// It reports a fatal test error if the returned status doesn't match expStatus or
// the returned RunConfig is nil when wantCfg is true or vice versa.
// The returned sig string can be used in test errors to describe the ParseArgs call.
func callParseArgsStdin(t *gotesting.T, baseArgs, passedArgs *Args, expStatus int, wantCfg bool) (
	cfg *RunConfig, stdout *bytes.Buffer, sig string) {
	if baseArgs == nil {
		baseArgs = &Args{}
	}
	stdin := &bytes.Buffer{}
	json.NewEncoder(stdin).Encode(passedArgs)
	stdout = &bytes.Buffer{}
	cfg, status := ParseArgs(nil, stdin, stdout, baseArgs, LocalRunner)
	sig = fmt.Sprintf("ParseArgs(nil, %v, ...)", *passedArgs)
	checkParseArgsResult(t, sig, cfg, status, expStatus, wantCfg)
	return cfg, stdout, sig
}

// callParseArgsCL calls ParseArgs with the supplied command-line arguments.
// Its behavior is otherwise similar to callParseArgsStdin.
func callParseArgsCL(t *gotesting.T, clArgs []string, expStatus int, wantCfg bool) (
	cfg *RunConfig, stdout *bytes.Buffer, sig string) {
	stdout = &bytes.Buffer{}
	cfg, status := ParseArgs(clArgs, &bytes.Buffer{}, stdout, &Args{}, LocalRunner)
	sig = fmt.Sprintf("ParseArgs(%v, ...)", clArgs)
	checkParseArgsResult(t, sig, cfg, status, expStatus, wantCfg)
	return cfg, stdout, sig
}

// checkParseArgsResult contains result-checking code shared between callParseArgsStdin and callParseArgsCL.
func checkParseArgsResult(t *gotesting.T, sig string, cfg *RunConfig, status, expStatus int, wantCfg bool) {
	if status != expStatus {
		t.Fatalf("%s returned status %v; want %v", sig, status, expStatus)
	} else if cfg == nil && wantCfg {
		t.Fatalf("%s returned nil RunConfig", sig)
	} else if cfg != nil && !wantCfg {
		t.Fatalf("%s returned non-nil RunConfig %v", sig, cfg)
	}
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
	args := Args{
		Mode:       ListTestsMode,
		BundleGlob: filepath.Join(dir, "*"),
	}
	_, b, sig := callParseArgsStdin(t, nil, &args, statusSuccess, false)
	var tests []*testing.Test
	if err := json.Unmarshal(b.Bytes(), &tests); err != nil {
		t.Fatalf("%s printed unparsable output %q", sig, b.String())
	}
	if len(tests) != 6 {
		t.Errorf("%s printed %v test(s); want 6: %v", sig, len(tests), b.String())
	}
}

func TestParseArgsSysInfo(t *gotesting.T) {
	td := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"logs/1.txt":    "first file",
		"crashes/1.dmp": "first crash",
	}); err != nil {
		t.Fatal(err)
	}

	// Get the initial state.
	baseArgs := Args{
		SystemLogDir:    filepath.Join(td, "logs"),
		SystemCrashDirs: []string{filepath.Join(td, "crashes")},
	}
	_, stdout, sig := callParseArgsStdin(t, &baseArgs, &Args{Mode: GetSysInfoStateMode}, 0, false)
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
	_, stdout, sig = callParseArgsStdin(t, &baseArgs, &args, 0, false)
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

	// RunTests should execute multiple test bundles and merge their output correctly.
	cfg, b, _ := callParseArgsStdin(t, nil, &Args{BundleGlob: filepath.Join(dir, "*")}, statusSuccess, true)

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

	msgs := readAllMessages(t, b)
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
	callParseArgsCL(t, []string{"-bogus"}, statusBadArgs, false)
}

func TestParseArgsNoBundles(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// ParseArgs should fail when a glob is passed that doesn't match any bundles.
	callParseArgsCL(t, []string{"-bundles=" + filepath.Join(dir, "bogus*")}, statusNoBundles, false)

	// When run by the tast executable, the command should exit with success but write a
	// RunError control message.
	_, b, sig := callParseArgsStdin(t, nil, &Args{BundleGlob: filepath.Join(dir, "bogus*")}, statusSuccess, false)
	if !gotRunError(readAllMessages(t, b)) {
		t.Fatalf("%s didn't write RunError message", sig)
	}
}

func TestRunTestsNoTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// RunTests should fail when run manually with a pattern is passed that doesn't match any tests.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), "bogus.SomeTest"}
	cfg, _, _ := callParseArgsCL(t, clArgs, statusSuccess, true)
	if status := RunTests(cfg); status != statusNoTests {
		t.Fatalf("RunTests(%v) = %v; want %v", cfg, status, statusNoTests)
	}

	// If the command was run by the tast command, it should exit with success.
	args := &Args{BundleGlob: filepath.Join(dir, "*"), Patterns: []string{"bogus.SomeTest"}}
	cfg, b, _ := callParseArgsStdin(t, nil, args, statusSuccess, true)
	if status := RunTests(cfg); status != statusSuccess {
		t.Fatalf("RunTests(%v) = %v; want %v", cfg, status, statusSuccess)
	}
	if gotRunError(readAllMessages(t, b)) {
		t.Fatalf("RunTests(%v) wrote RunError message", cfg)
	}
}

func TestRunTestsFailForErrorWhenRunManually(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true, false, true})
	defer os.RemoveAll(dir)

	// RunTests should fail when a bundle reports failure while run manually.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*")}
	cfg, _, _ := callParseArgsCL(t, clArgs, statusSuccess, true)
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
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), getTestName(0, 0)}
	cfg, _, _ := callParseArgsCL(t, clArgs, statusSuccess, true)
	if status := RunTests(cfg); status != statusSuccess {
		t.Fatalf("RunConfig(%v) = %v; want %v", cfg, status, statusSuccess)
	}
}

func TestRunTestsUseRequestedOutDir(t *gotesting.T) {
	bundleDir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(bundleDir)
	outDir := testutil.TempDir(t, "runner_test.")
	defer os.RemoveAll(outDir)

	cfg, b, _ := callParseArgsStdin(t, nil, &Args{BundleGlob: filepath.Join(bundleDir, "*"), OutDir: outDir}, statusSuccess, true)
	if status := RunTests(cfg); status != statusSuccess {
		t.Fatalf("RunConfig(%v) = %v; want %v", cfg, status, statusSuccess)
	}
	msgs := readAllMessages(t, b)
	if re, ok := msgs[len(msgs)-1].(*control.RunEnd); !ok {
		t.Errorf("Last message not RunEnd: %v", msgs[len(msgs)-1])
	} else {
		// The RunEnd message should contain the out dir that we originally requested.
		if re.OutDir != outDir {
			t.Errorf("RunEnd.OutDir = %q; want %q", re.OutDir, outDir)
		}
	}
}
