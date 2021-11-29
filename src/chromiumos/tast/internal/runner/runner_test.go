// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"context"
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

	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

const (
	// Prefix for bundles created by createBundleSymlinks.
	bundlePrefix = "fake_bundle"
)

var (
	// fakeFixture1 and fakeFixture2 are fake fixtures which fake bundles have.
	fakeFixture1 = &testing.FixtureInstance{Name: "fake1"}
	fakeFixture2 = &testing.FixtureInstance{Name: "fake2", Parent: "fake1"}
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

	dir = testutil.TempDir(t)
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
	bundleName := filepath.Base(os.Args[0])

	parts := strings.Split(bundleName, "-")
	if len(parts) != 3 {
		log.Fatalf("Unparsable filename %q", os.Args[0])
	}
	bundleNum, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Fatalf("Bad bundle number %q in filename %q", parts[1], os.Args[0])
	}

	reg := testing.NewRegistry(bundleName)
	reg.AddFixtureInstance(fakeFixture1)
	reg.AddFixtureInstance(fakeFixture2)

	for i, res := range parts[2] {
		var f testing.TestFunc
		if res == 'p' {
			f = func(context.Context, *testing.State) {}
		} else if res == 'f' {
			f = func(ctx context.Context, s *testing.State) { s.Fatal("Failed") }
		} else {
			log.Fatalf("Bad rune %v in result string %q", res, parts[2])
		}
		reg.AddTestInstance(&testing.TestInstance{
			Name: getTestName(bundleNum, i),
			Func: f,
		})
	}

	return bundle.Local(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, reg, bundle.Delegate{})
}

// newBufferWithArgs returns a buffer containing the JSON representation of args.
func newBufferWithArgs(t *gotesting.T, args *jsonprotocol.RunnerArgs) *bytes.Buffer {
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
func callRun(t *gotesting.T, clArgs []string, stdinArgs *jsonprotocol.RunnerArgs, scfg *StaticConfig) (
	status int, stdout, stderr *bytes.Buffer, sig string) {
	sig = fmt.Sprintf("Run(%v, %+v)", clArgs, stdinArgs)
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	return Run(clArgs, newBufferWithArgs(t, stdinArgs), stdout, stderr, scfg), stdout, stderr, sig
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

func TestRunTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{false, true}, []bool{true})
	defer os.RemoveAll(dir)

	// Run should execute multiple test bundles and merge their output correctly.
	args := &jsonprotocol.RunnerArgs{RunTests: &jsonprotocol.RunnerRunTestsArgs{BundleGlob: filepath.Join(dir, "*")}}
	status, stdout, stderr, sig := callRun(t, nil, args, &StaticConfig{Type: LocalRunner})
	if status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}

	msgs := readAllMessages(t, stdout)
	if rs, ok := msgs[0].(*control.RunStart); !ok {
		t.Errorf("First message not RunStart: %v", msgs[0])
	} else {
		// Construct names for both tests in bundle 0 and the single test in bundle 1.
		expNames := []string{getTestName(0, 0), getTestName(0, 1), getTestName(1, 0)}
		if !reflect.DeepEqual(rs.TestNames, expNames) {
			t.Errorf("RunStart included test names %v; want %v", rs.TestNames, expNames)
		}
		if rs.NumTests != 3 {
			t.Errorf("RunStart reported %v test(s); want 3", rs.NumTests)
		}
	}
	if re, ok := msgs[len(msgs)-1].(*control.RunEnd); !ok {
		t.Errorf("Last message not RunEnd: %v", msgs[len(msgs)-1])
	} else {
		if fi, err := os.Stat(re.OutDir); os.IsNotExist(err) {
			t.Errorf("RunEnd out dir %q doesn't exist", re.OutDir)
		} else {
			if mode := fi.Mode() & os.ModePerm; mode != 0755 {
				t.Errorf("Out dir %v has mode 0%o; want 0%o", re.OutDir, mode, 0755)
			}
			os.RemoveAll(re.OutDir)
		}
	}

	// Check that the right number of tests are reported as started and failed and that bundles and tests are sorted.
	// (Bundle names are included in these test names, so we can verify sorting by just comparing names.)
	tests := make(map[string]struct{})
	failed := make(map[string]struct{})
	var name string
	for _, msg := range msgs {
		if ts, ok := msg.(*control.EntityStart); ok {
			if ts.Info.Name < name {
				t.Errorf("Saw unsorted test %q after %q", ts.Info.Name, name)
			}
			name = ts.Info.Name
			tests[name] = struct{}{}
		} else if _, ok := msg.(*control.EntityError); ok {
			failed[name] = struct{}{}
		}
	}
	if len(tests) != 3 {
		t.Errorf("Got EntityStart messages for %v test(s); want 3", len(tests))
	}
	if len(failed) != 1 {
		t.Errorf("Got EntityError messages for %v test(s); want 1", len(failed))
	}

	if stderr.Len() != 0 {
		t.Errorf("%s wrote %q to stderr", sig, stderr.String())
	}
}

func TestInvalidFlag(t *gotesting.T) {
	status, stdout, stderr, sig := callRun(t, []string{"-bogus"}, nil, &StaticConfig{Type: LocalRunner})
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
	status, stdout, stderr, sig := callRun(t, clArgs, nil, &StaticConfig{Type: LocalRunner})
	if status == 0 {
		t.Errorf("%s = %v; want non-zero", sig, status)
	}
	if stdout.Len() != 0 {
		t.Errorf("%s wrote %q to stdout; want nothing (error should go to stderr)", sig, stdout.String())
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}

	// If the command was run by the tast command, it should exit with success.
	args := &jsonprotocol.RunnerArgs{
		RunTests: &jsonprotocol.RunnerRunTestsArgs{
			BundleArgs: jsonprotocol.BundleRunTestsArgs{Patterns: []string{"bogus.SomeTest"}},
			BundleGlob: filepath.Join(dir, "*"),
		},
	}
	scfg := &StaticConfig{Type: LocalRunner}
	if status, stdout, stderr, sig = callRun(t, nil, args, scfg); status != statusSuccess {
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
	status, _, stderr, sig := callRun(t, clArgs, nil, &StaticConfig{Type: LocalRunner})
	if status != statusTestFailed {
		t.Errorf("%s = %v; want %v", sig, status, statusTestFailed)
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}
}

func TestSkipBundlesWithoutMatchedTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true}, []bool{true})
	defer os.RemoveAll(dir)

	// Bundles report failure if instructed to run using a pattern that doesn't
	// match any tests in the bundle. Pass the name of the test in the first bundle and
	// check that the run succeeds (i.e. the second bundle wasn't executed).
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), getTestName(0, 0)}
	status, _, _, sig := callRun(t, clArgs, nil, &StaticConfig{Type: LocalRunner})
	if status != statusSuccess {
		t.Errorf("%s = %v; want %v", sig, status, statusSuccess)
	}
}

func TestRunTestsUseRequestedOutDir(t *gotesting.T) {
	bundleDir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(bundleDir)
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	status, stdout, _, sig := callRun(t, nil, &jsonprotocol.RunnerArgs{
		RunTests: &jsonprotocol.RunnerRunTestsArgs{
			BundleGlob: filepath.Join(bundleDir, "*"),
			BundleArgs: jsonprotocol.BundleRunTestsArgs{OutDir: outDir},
		},
	}, &StaticConfig{Type: LocalRunner})
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
