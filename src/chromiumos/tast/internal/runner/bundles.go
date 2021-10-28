// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/unix"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// getBundlesAndTests returns matched tests and paths to the bundles containing them.
func getBundlesAndTests(args *jsonprotocol.RunnerArgs) (bundles []string, tests []*jsonprotocol.EntityWithRunnabilityInfo, err *command.StatusError) {
	var glob string
	switch args.Mode {
	case jsonprotocol.RunnerRunTestsMode:
		glob = args.RunTests.BundleGlob
	case jsonprotocol.RunnerListTestsMode:
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
func getTests(args *jsonprotocol.RunnerArgs, bundles []string) (tests []*jsonprotocol.EntityWithRunnabilityInfo,
	bundlesWithTests []string, statusErr *command.StatusError) {
	bundleArgs, err := args.BundleArgs(jsonprotocol.BundleListTestsMode)
	if err != nil {
		return nil, nil, command.NewStatusErrorf(statusBadArgs, "%v", err)
	}

	matcher, err := testing.NewMatcher(bundleArgs.ListTests.Patterns)
	if err != nil {
		return nil, nil, command.NewStatusErrorf(statusBadArgs, "%v", err)
	}

	// Run all bundles in parallel.
	ch := make(chan testsOrError, len(bundles))
	for _, b := range bundles {
		bundle := b
		go func() {
			tests, err := func() ([]*jsonprotocol.EntityWithRunnabilityInfo, *command.StatusError) {
				ctx := context.Background()

				conn, err := rpc.DialExec(ctx, bundle, 0, false, &protocol.HandshakeRequest{})
				if err != nil {
					return nil, command.NewStatusErrorf(statusBundleFailed, "failed to connect to bundle %s: %v", bundle, err)
				}
				defer conn.Close()

				cl := protocol.NewTestServiceClient(conn.Conn())

				res, err := cl.ListEntities(ctx, &protocol.ListEntitiesRequest{Features: bundleArgs.ListTests.FeatureArgs.Features()})
				if err != nil {
					return nil, command.NewStatusErrorf(statusBundleFailed, "failed to list entities in bundle %s: %v", bundle, err)
				}

				var tests []*jsonprotocol.EntityWithRunnabilityInfo
				for _, e := range res.Entities {
					if e.GetEntity().GetType() != protocol.EntityType_TEST {
						continue
					}
					if !matcher.Match(e.GetEntity().GetName(), e.GetEntity().GetAttributes()) {
						continue
					}
					tests = append(tests, jsonprotocol.MustEntityWithRunnabilityInfoFromProto(e))
				}
				return tests, nil
			}()
			ch <- testsOrError{bundle, tests, err}
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

	// Run all the bundles in parallel.
	ch := make(chan *fixturesOrError, len(bundles))
	for _, bundle := range bundles {
		bundle := bundle
		go func() {
			fs, err := func() ([]*jsonprotocol.EntityInfo, *command.StatusError) {
				ctx := context.Background()

				conn, err := rpc.DialExec(ctx, bundle, 0, false, &protocol.HandshakeRequest{})
				if err != nil {
					return nil, command.NewStatusErrorf(statusBundleFailed, "failed to connect to bundle %s: %v", bundle, err)
				}
				defer conn.Close()

				cl := protocol.NewTestServiceClient(conn.Conn())

				res, err := cl.ListEntities(ctx, &protocol.ListEntitiesRequest{})
				if err != nil {
					return nil, command.NewStatusErrorf(statusBundleFailed, "failed to list entities in bundle %s: %v", bundle, err)
				}

				var fs []*jsonprotocol.EntityInfo
				for _, e := range res.Entities {
					if e.GetEntity().GetType() != protocol.EntityType_FIXTURE {
						continue
					}
					fs = append(fs, jsonprotocol.MustEntityInfoFromProto(e.GetEntity()))
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

// killSession makes a best-effort attempt to kill all processes in session sid.
// It makes several passes over the list of running processes, sending sig to any
// that are part of the session. After it doesn't find any new processes, it returns.
// Note that this is racy: it's possible (but hopefully unlikely) that continually-forking
// processes could spawn children that don't get killed.
func killSession(sid int, sig unix.Signal) {
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
				unix.Kill(pid, sig)
				n++
			}
		}
		// If we didn't find any processes in the session, we're done.
		if n == 0 {
			return
		}
	}
}

// handleDownloadPrivateBundles handles a RunnerDownloadPrivateBundlesMode request from args
// and JSON-marshals a RunnerDownloadPrivateBundlesResult struct to w.
func handleDownloadPrivateBundles(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, stdout io.Writer) error {
	var logs []string
	logger := logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(func(msg string) {
		logs = append(logs, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), msg))
	}))
	ctx = logging.AttachLogger(ctx, logger)

	compat, err := startCompatServer(ctx, scfg, &protocol.HandshakeRequest{})
	if err != nil {
		return err
	}
	defer compat.Close()

	res, err := compat.Client().DownloadPrivateBundles(ctx, args.DownloadPrivateBundles.Proto())
	if err != nil {
		return err
	}

	jres := jsonprotocol.RunnerDownloadPrivateBundlesResultFromProto(res)
	jres.Messages = logs

	if err := json.NewEncoder(stdout).Encode(jres); err != nil {
		return command.NewStatusErrorf(statusError, "failed to serialize into JSON: %v", err)
	}
	return nil
}
