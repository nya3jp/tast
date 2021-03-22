// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testcontext"
)

// getBundlesAndTests returns matched tests and paths to the bundles containing them.
func getBundlesAndTests(args *Args) (bundles []string, tests []*jsonprotocol.EntityWithRunnabilityInfo, err *command.StatusError) {
	var glob string
	switch args.Mode {
	case RunTestsMode:
		glob = args.RunTests.BundleGlob
	case ListTestsMode:
		glob = args.ListTests.BundleGlob
	default:
		return nil, nil, command.NewStatusErrorf(statusBadArgs, "bundles unneeded for mode %v", args.Mode)
	}

	if bundles, err = getBundles(glob); err != nil {
		return nil, nil, err
	}
	tests, bundles, err = getTests(args, bundles)
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
	tests  []*jsonprotocol.EntityWithRunnabilityInfo
	err    *command.StatusError
}

// getTests returns tests in bundles matched by args.Patterns. It does this by executing
// each bundle to ask it to marshal and print its tests. A slice of paths to bundles
// with matched tests is also returned.
func getTests(args *Args, bundles []string) (tests []*jsonprotocol.EntityWithRunnabilityInfo,
	bundlesWithTests []string, statusErr *command.StatusError) {
	bundleArgs, err := args.bundleArgs(bundle.ListTestsMode)
	if err != nil {
		return nil, nil, command.NewStatusErrorf(statusBadArgs, "%v", err)
	}

	// Run all bundles in parallel.
	ch := make(chan testsOrError, len(bundles))
	for _, b := range bundles {
		bundle := b
		go func() {
			out := bytes.Buffer{}
			if err := runBundle(bundle, bundleArgs, &out); err != nil {
				ch <- testsOrError{bundle, nil, err}
				return
			}
			ts := make([]*jsonprotocol.EntityWithRunnabilityInfo, 0)
			if err := json.Unmarshal(out.Bytes(), &ts); err != nil {
				ch <- testsOrError{bundle, nil,
					command.NewStatusErrorf(statusBundleFailed, "bundle %v gave bad output: %v", bundle, err)}
				return
			}
			ch <- testsOrError{bundle, ts, nil}
		}()
	}

	// Read results into a map from bundle to that bundle's tests.
	bundleTests := make(map[string][]*jsonprotocol.EntityWithRunnabilityInfo)
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

// listFixtures returns listFixtures in bundles. It does this by executing
// each bundle to ask it to marshal and print them.
func listFixtures(bundleGlob string) (map[string][]*jsonprotocol.EntityInfo, *command.StatusError) {
	type fixturesOrError struct {
		bundle string
		fs     []*jsonprotocol.EntityInfo
		err    *command.StatusError
	}

	bundles, err := getBundles(bundleGlob)
	if err != nil {
		return nil, err
	}

	bundleArgs := &bundle.Args{
		Mode: bundle.ListFixturesMode,
	}
	// Run all the bundles in parallel.
	ch := make(chan *fixturesOrError, len(bundles))
	for _, bundle := range bundles {
		bundle := bundle
		go func() {
			out := bytes.Buffer{}
			if err := runBundle(bundle, bundleArgs, &out); err != nil {
				ch <- &fixturesOrError{bundle, nil, err}
				return
			}

			fs, err := func() ([]*jsonprotocol.EntityInfo, *command.StatusError) {
				var fs []*jsonprotocol.EntityInfo
				if err := json.Unmarshal(out.Bytes(), &fs); err != nil {
					return nil, command.NewStatusErrorf(statusBundleFailed, "bundle %v gave bad output: %v", bundle, err)
				}
				return fs, nil
			}()
			ch <- &fixturesOrError{bundle, fs, err}
		}()
	}

	bundleFixts := make(map[string][]*jsonprotocol.EntityInfo)
	for i := 0; i < len(bundles); i++ {
		foe := <-ch
		if foe.err != nil {
			return nil, foe.err
		}
		if len(foe.fs) > 0 {
			bundleFixts[foe.bundle] = foe.fs
		}
	}
	return bundleFixts, nil
}

// startBundleCmd creates and returns a new command running the test bundle at path using args.
// cmd's Start method has already been called, and the caller is responsible for calling Wait.
// A new session is created for the bundle process.
func startBundleCmd(path string, bundleArgs *bundle.Args, stdout, stderr io.Writer) (*exec.Cmd, error) {
	var stdin bytes.Buffer
	if err := json.NewEncoder(&stdin).Encode(bundleArgs); err != nil {
		return nil, err
	}

	cmd := exec.Command(path)
	cmd.Stdin = &stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// Run the bundle in a new session so we can identify test processes later.
	// We can't just use a process group here, as the testexec package places each command
	// run by a test into its own process group.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// runBundle runs the bundle at path to completion, passing bundleArgs.
// The bundle's stdout is copied to the stdout arg.
func runBundle(path string, bundleArgs *bundle.Args, stdout io.Writer) *command.StatusError {
	// Watch for stdout being closed so we can abort the bundle and clean up: https://crbug.com/945626
	// Otherwise, the runner, bundle, and processes started by tests may run indefinitely.
	// When stdout is closed, it's important that we clean up before writing anything to it, as Go will
	// terminate the process if SIGPIPE is generated by a write to a closed stdout/stderr.
	// See https://golang.org/pkg/os/signal/#hdr-SIGPIPE for more details.
	stdoutWatcher, err := newPipeWatcher(int(os.Stdout.Fd()))
	if err != nil {
		return command.NewStatusErrorf(statusError, "failed watching stdout: %v", err)
	}
	defer stdoutWatcher.close()

	// Also catch SIGINT so we can clean up if the runner was executed manually and
	// later interrupted with Ctrl-C, and SIGTERM in case we're killed by another runner process.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	stderr := bytes.Buffer{}
	cmd, err := startBundleCmd(path, bundleArgs, stdout, &stderr)
	if err != nil {
		return command.NewStatusErrorf(statusBundleFailed, "%v", err)
	}

	// When we return, kill the bundle process and any other processes in its session.
	// Per setsid(2), "[the calling process's] session ID is made the same as its process ID".
	defer killSession(cmd.Process.Pid, syscall.SIGKILL)

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		// The bundle process exited on its own.
		if err == nil {
			return nil
		}
		var detail string
		if msg := strings.TrimSpace(stderr.String()); len(msg) > 0 {
			detail = fmt.Sprintf(" (%v)", msg)
		}
		return command.NewStatusErrorf(statusBundleFailed, "%v%s", err, detail)
	case <-stdoutWatcher.readClosed:
		// The read end of stdout was closed (i.e. the shell or SSH connection used to run us died).
		return command.NewStatusErrorf(statusInterrupted, "stdout closed")
	case sig := <-sigCh:
		// We caught SIGINT (i.e. we were run manually and the user hit Ctrl-C) or
		// SIGTERM (we were likely killed by another test runner process).
		status := statusError
		switch sig {
		case syscall.SIGINT:
			status = statusInterrupted
		case syscall.SIGTERM:
			status = statusTerminated
		}
		return command.NewStatusErrorf(status, "caught signal %d (%s)", sig, sig)
	}
}

// killSession makes a best-effort attempt to kill all processes in session sid.
// It makes several passes over the list of running processes, sending sig to any
// that are part of the session. After it doesn't find any new processes, it returns.
// Note that this is racy: it's possible (but hopefully unlikely) that continually-forking
// processes could spawn children that don't get killed.
func killSession(sid int, sig syscall.Signal) {
	const maxPasses = 3
	for i := 0; i < maxPasses; i++ {
		procs, err := process.Processes()
		if err != nil {
			return
		}
		n := 0
		for _, proc := range procs {
			pid := int(proc.Pid)
			if s, err := unix.Getsid(pid); err == nil && s == sid {
				syscall.Kill(pid, sig)
				n++
			}
		}
		// If we didn't find any processes in the session, we're done.
		if n == 0 {
			return
		}
	}
}

// handleDownloadPrivateBundles handles a DownloadPrivateBundlesMode request from args
// and JSON-marshals a DownloadPrivateBundlesResult struct to w.
func handleDownloadPrivateBundles(ctx context.Context, args *Args, cfg *Config, stdout io.Writer) error {
	if cfg.PrivateBundlesStampPath == "" {
		return errors.New("this test runner is not configured for private bundles")
	}

	if args.DownloadPrivateBundles.BuildArtifactsURL == "" {
		return errors.New("failed to determine the build artifacts URL (non-official image?)")
	}

	var logs []string
	var mu sync.Mutex
	ctx = testcontext.WithLogger(ctx, func(msg string) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), msg))
	})

	defer func() {
		res := &DownloadPrivateBundlesResult{Messages: logs}
		json.NewEncoder(stdout).Encode(res)
	}()

	// If the stamp file exists, private bundles have been already downloaded.
	if _, err := os.Stat(cfg.PrivateBundlesStampPath); err == nil {
		return nil
	}

	// Download the archive via devserver.
	archiveURL := args.DownloadPrivateBundles.BuildArtifactsURL + "tast_bundles.tar.bz2"
	testcontext.Logf(ctx, "Downloading private bundles from %s", archiveURL)
	cl, err := devserver.NewClient(ctx, args.DownloadPrivateBundles.Devservers,
		args.DownloadPrivateBundles.TLWServer,
		args.DownloadPrivateBundles.DUTName)
	if err != nil {
		return errors.Wrapf(err, "failed to create new client [devservers=%v, TLWServer=%s]",
			args.DownloadPrivateBundles.Devservers, args.DownloadPrivateBundles.TLWServer)
	}
	defer cl.TearDown()

	r, err := cl.Open(ctx, archiveURL)
	if err != nil {
		return err
	}
	defer r.Close()

	tf, err := ioutil.TempFile("", "tast_bundles.")
	if err != nil {
		return err
	}
	defer os.Remove(tf.Name())

	_, err = io.Copy(tf, r)

	if cerr := tf.Close(); err == nil {
		err = cerr
	}

	if err == nil {
		// Extract the archive, and touch the stamp file.
		cmd := exec.Command("tar", "xf", tf.Name())
		cmd.Dir = "/usr/local"
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to extract %s: %v", strings.Join(cmd.Args, " "), err)
		}
		testcontext.Log(ctx, "Download finished successfully")
	} else if os.IsNotExist(err) {
		testcontext.Log(ctx, "Private bundles not found")
	} else {
		return fmt.Errorf("failed to download %s: %v", archiveURL, err)
	}

	return ioutil.WriteFile(cfg.PrivateBundlesStampPath, nil, 0644)
}
