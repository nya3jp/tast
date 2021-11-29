// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	gotesting "testing"

	"chromiumos/tast/internal/bundle"
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

// callRun calls Run using the supplied arguments and returns its output, status, and a signature that can
// be used in error messages.
func callRun(clArgs []string, scfg *StaticConfig) (status int, stdout, stderr *bytes.Buffer, sig string) {
	sig = fmt.Sprintf("Run(%v)", clArgs)
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	return Run(clArgs, &bytes.Buffer{}, stdout, stderr, scfg), stdout, stderr, sig
}

func TestRun_DeprecatedDirectRun(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true, true}, []bool{true})
	defer os.RemoveAll(dir)

	// Run should execute multiple test bundles and merge their output correctly.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*")}
	status, stdout, stderr, sig := callRun(clArgs, &StaticConfig{Type: LocalRunner})

	if status != statusSuccess {
		t.Errorf("%s = %v; want %v", sig, status, statusSuccess)
	}

	const want = "Ran 3 test(s)"
	logs := stdout.String()
	if !strings.Contains(logs, want) {
		t.Errorf("%q not found in logs", want)
	}

	if stderr.Len() != 0 {
		t.Errorf("%s wrote %q to stderr", sig, stderr.String())
	}
}

func TestRun_DeprecatedDirectRun_InvalidFlag(t *gotesting.T) {
	status, stdout, stderr, sig := callRun([]string{"-bogus"}, &StaticConfig{Type: LocalRunner})
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

func TestRun_DeprecatedDirectRun_NoTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(dir)

	// RunTests should fail when run manually with a pattern is passed that doesn't match any tests.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), "bogus.SomeTest"}
	status, stdout, stderr, sig := callRun(clArgs, &StaticConfig{Type: LocalRunner})
	if status == 0 {
		t.Errorf("%s = %v; want non-zero", sig, status)
	}
	if stdout.Len() != 0 {
		t.Errorf("%s wrote %q to stdout; want nothing (error should go to stderr)", sig, stdout.String())
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}
}

func TestRun_DeprecatedDirectRun_FailForTestError(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true, false, true})
	defer os.RemoveAll(dir)

	// RunTests should fail when a bundle reports failure while run manually.
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*")}
	status, _, stderr, sig := callRun(clArgs, &StaticConfig{Type: LocalRunner})
	if status != statusTestFailed {
		t.Errorf("%s = %v; want %v", sig, status, statusTestFailed)
	}
	if stderr.Len() == 0 {
		t.Errorf("%s didn't write error to stderr", sig)
	}
}

func TestRun_DeprecatedDirectRun_SkipBundlesWithoutMatchedTests(t *gotesting.T) {
	dir := createBundleSymlinks(t, []bool{true}, []bool{true})
	defer os.RemoveAll(dir)

	// Bundles report failure if instructed to run using a pattern that doesn't
	// match any tests in the bundle. Pass the name of the test in the first bundle and
	// check that the run succeeds (i.e. the second bundle wasn't executed).
	clArgs := []string{"-bundles=" + filepath.Join(dir, "*"), getTestName(0, 0)}
	status, _, _, sig := callRun(clArgs, &StaticConfig{Type: LocalRunner})
	if status != statusSuccess {
		t.Errorf("%s = %v; want %v", sig, status, statusSuccess)
	}
}

func TestRun_DeprecatedDirectRun_UseRequestedOutDir(t *gotesting.T) {
	bundleDir := createBundleSymlinks(t, []bool{true})
	defer os.RemoveAll(bundleDir)
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	clArgs := []string{
		"-bundles=" + filepath.Join(bundleDir, "*"),
		"-outdir=" + outDir,
	}
	status, _, _, sig := callRun(clArgs, &StaticConfig{Type: LocalRunner})
	if status != statusSuccess {
		t.Fatalf("%s = %v; want %v", sig, status, statusSuccess)
	}

	testOutDir := filepath.Join(outDir, getTestName(0, 0))
	if _, err := os.Stat(testOutDir); err != nil {
		t.Errorf("%s doesn't exist: %v", testOutDir, err)
	}
}
