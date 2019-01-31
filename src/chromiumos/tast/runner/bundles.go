// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"chromiumos/tast/bundle"
	"chromiumos/tast/command"
	"chromiumos/tast/testing"
)

// getBundlesAndTests returns matched tests and paths to the bundles containing them.
func getBundlesAndTests(args *Args) (bundles []string, tests []*testing.Test, err *command.StatusError) {
	if bundles, err = getBundles(args.BundleGlob); err != nil {
		return nil, nil, err
	}
	tests, bundles, err = getTests(bundles, args.bundleArgs)
	return bundles, tests, err
}

// getBundles returns the full paths of all test bundles matched by glob.
func getBundles(glob string) ([]string, *command.StatusError) {
	ps, err := filepath.Glob(glob)
	if err != nil {
		return nil, command.NewStatusErrorf(statusNoBundles, "failed to get bundle(s) %q: %v", glob, err)
	}

	bundles := make([]string, 0)
	for _, p := range ps {
		fi, err := os.Stat(p)
		// Only match executable regular files.
		if err == nil && fi.Mode().IsRegular() && (fi.Mode().Perm()&0111) != 0 {
			bundles = append(bundles, p)
		}
	}
	if len(bundles) == 0 {
		return nil, command.NewStatusErrorf(statusNoBundles, "no bundles matched by %q", glob)
	}
	sort.Strings(bundles)
	return bundles, nil
}

type testsOrError struct {
	bundle string
	tests  []*testing.Test
	err    *command.StatusError
}

// getTests returns tests in bundles matched by args.Patterns. It does this by executing
// each bundle to ask it to marshal and print its tests. A slice of paths to bundles
// with matched tests is also returned.
func getTests(bundles []string, args bundle.Args) (tests []*testing.Test, bundlesWithTests []string, err *command.StatusError) {
	args.Mode = bundle.ListTestsMode

	// Run all bundles in parallel.
	ch := make(chan testsOrError, len(bundles))
	for _, b := range bundles {
		bundle := b
		go func() {
			out := bytes.Buffer{}
			if err := runBundle(bundle, &args, &out); err != nil {
				ch <- testsOrError{bundle, nil, err}
				return
			}
			ts := make([]*testing.Test, 0)
			if err := json.Unmarshal(out.Bytes(), &ts); err != nil {
				ch <- testsOrError{bundle, nil,
					command.NewStatusErrorf(statusBundleFailed, "bundle %v gave bad output: %v", bundle, err)}
				return
			}
			ch <- testsOrError{bundle, ts, nil}
		}()
	}

	// Read results into a map from bundle to that bundle's tests.
	bundleTests := make(map[string][]*testing.Test)
	for i := 0; i < len(bundles); i++ {
		toe := <-ch
		if toe.err != nil {
			return nil, nil, toe.err
		}
		if len(toe.tests) > 0 {
			bundleTests[toe.bundle] = toe.tests
		}
	}

	// Sort both the tests and the bundles by bundle path.
	for b := range bundleTests {
		bundlesWithTests = append(bundlesWithTests, b)
	}
	sort.Strings(bundlesWithTests)
	for _, b := range bundlesWithTests {
		tests = append(tests, bundleTests[b]...)
	}
	return tests, bundlesWithTests, nil
}

// runBundle runs the bundle at path to completion, passing args.
// The bundle's stdout is copied to the stdout arg.
func runBundle(path string, args *bundle.Args, stdout io.Writer) *command.StatusError {
	cmd := exec.Command(path)
	cmd.Stdout = stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	// Save stderr so we can return it to aid in debugging.
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		return command.NewStatusErrorf(statusBundleFailed, "%v", err)
	}

	jerr := json.NewEncoder(stdin).Encode(args)
	stdin.Close()
	err = cmd.Wait()

	if jerr != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}
	if err != nil {
		// Include stderr if the bundle wrote anything to it.
		var detail string
		if msg := strings.TrimSpace(stderr.String()); len(msg) > 0 {
			detail = fmt.Sprintf(" (%v)", msg)
		}
		return command.NewStatusErrorf(statusBundleFailed, "%v%s", err, detail)
	}
	return nil
}

// handleDownloadPrivateBundles handles a DownloadPrivateBundlesMode request from args
// and JSON-marshals a DownloadPrivateBundlesResult struct to w.
func handleDownloadPrivateBundles(args *Args, stdout io.Writer) error {
	const (
		archiveName = "tast_bundles.tar.gz"
		stampPath   = "/usr/local/share/tast/.private-bundle-downloaded"
	)

	// If the stamp file exists, private bundles have been already downloaded.
	if _, err := os.Stat(stampPath); err == nil {
		res := &DownloadPrivateBundlesResult{Logs: []string{"Private bundles up-to-date"}}
		return json.NewEncoder(stdout).Encode(res)
	}

	// Build the private bundle archive URL.
	if args.BuilderPath == "" {
		var err error
		args.BuilderPath, err = readBuilderPath()
		if err != nil {
			return fmt.Errorf("failed to determine builder path: %v", err)
		}
	}
	gsURL := fmt.Sprintf("gs://chromeos-image-archive/%s/%s", args.BuilderPath, archiveName)

	// Download the archive via devserver.
	var logs []string
	var mu sync.Mutex
	lf := func(msg string) {
		mu.Lock()
		logs = append(logs, msg)
		mu.Unlock()
	}

	// TODO(nya): Consider applying timeout.
	ctx := context.TODO()
	cl := newDevserverClient(ctx, args.Devservers, lf)

	tf, err := ioutil.TempFile("", archiveName+".")
	if err != nil {
		return err
	}
	defer os.Remove(tf.Name())

	_, err = cl.DownloadGS(ctx, tf, gsURL)
	tf.Close()
	if err != nil {
		return fmt.Errorf("failed to download %s: %v", archiveName, err)
	}

	// Extract the archive, and create the stamp file.
	cmd := exec.Command("tar", "xf", tf.Name())
	cmd.Dir = "/usr/local"
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract %s: %v", strings.Join(cmd.Args, " "), err)
	}

	if err := ioutil.WriteFile(stampPath, nil, 0666); err != nil {
		return err
	}

	res := &DownloadPrivateBundlesResult{Logs: logs}
	return json.NewEncoder(stdout).Encode(res)
}

// readBuilderPath returns the URL of the private bundle archive on
// Google Cloud Storage.
func readBuilderPath() (string, error) {
	const (
		path = "/etc/lsb-release"
		key  = "CHROMEOS_RELEASE_BUILDER_PATH"
	)

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		v := strings.SplitN(sc.Text(), "=", 2)
		if len(v) == 2 && v[0] == key {
			url := fmt.Sprintf("gs://chromeos-image-archive/%s/tast_bundles.tar.gz", v[1])
			return url, nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s not found in %s", key, path)
}
