// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/prepare"
	"chromiumos/tast/cmd/tast/internal/run/sharding"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/devserver"
	"chromiumos/tast/internal/run/reporting"
	"chromiumos/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/xcontext"
)

const (
	// maxPostReserve is maximum amount of time reserved for post-processing
	// (e.g. writing results and collecting system info).
	maxPostReserve = 15 * time.Second

	// DUTInfoFile is a file name containing the dump of obtained DUTInfo message,
	// which is directly under ResDir.
	DUTInfoFile = "dut-info.txt"
)

// Run executes or lists tests per cfg and returns the results.
// Messages are logged via ctx as the run progresses.
func Run(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]*resultsjson.Result, error) {
	reportClient, err := reporting.NewRPCClient(ctx, cfg.ReportsServer())
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up gRPC servers")
	}
	defer reportClient.Close()

	// Always start an ephemeral devserver for remote tests if TLWServer is not specified, and allowed.
	if cfg.TLWServer() == "" && cfg.UseEphemeralDevserver() {
		es, err := startEphemeralDevserverForRemoteTests(ctx, cfg, state)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start ephemeral devserver for remote tests")
		}
		defer es.Close()
	} else {
		state.RemoteDevservers = cfg.Devservers()
	}

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to target")
	}
	defer drv.Close(ctx)

	dutInfo, err := prepare.Prepare(ctx, cfg, drv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build and push")
	}

	switch cfg.Mode() {
	case config.ListTestsMode:
		results, err := listTests(ctx, cfg, drv, dutInfo)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list tests")
		}
		return results, nil
	case config.RunTestsMode:
		results, err := runTests(ctx, cfg, state, drv, reportClient, dutInfo)
		if err != nil {
			return results, errors.Wrapf(err, "failed to run tests")
		}
		return results, nil
	default:
		return nil, errors.Errorf("unhandled mode %d", cfg.Mode())
	}
}

// startEphemeralDevserverForRemoteTests starts an ephemeral devserver for remote tests.
func startEphemeralDevserverForRemoteTests(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) (*devserver.Ephemeral, error) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen to a local port: %v", err)
	}

	cacheDir := filepath.Join(cfg.TastDir(), "devserver", "static")
	es, err := devserver.NewEphemeral(lis, cacheDir, cfg.ExtraAllowedBuckets())
	if err != nil {
		return nil, err
	}

	state.RemoteDevservers = []string{fmt.Sprintf("http://%s", lis.Addr())}
	logging.Info(ctx, "Starting ephemeral devserver at ", state.RemoteDevservers[0], " for remote tests")
	return es, nil
}

func removeSkippedTestsFromBundle(bundle []*driver.BundleEntity) ([]*driver.BundleEntity, []*driver.BundleEntity) {
	var filteredBundle []*driver.BundleEntity
	var skippedBundle []*driver.BundleEntity
	for _, re := range bundle {
		// Guard clause to not add test that would be skipped
		// if the ExcludeSkipped flag is set
		if len(re.Resolved.GetSkip().GetReasons()) > 0 {
			skippedBundle = append(skippedBundle, re)
			continue
		}

		filteredBundle = append(filteredBundle, re)
	}

	return filteredBundle, skippedBundle
}

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *config.Config, drv *driver.Driver, dutInfo *protocol.DUTInfo) ([]*resultsjson.Result, error) {
	tests, err := drv.ListMatchedTests(ctx, cfg.Features(dutInfo.GetFeatures()))
	if err != nil {
		return nil, err
	}

	shard := sharding.Compute(tests, cfg.ShardIndex(), cfg.TotalShards())

	var testsToPrint []*driver.BundleEntity
	if cfg.ExcludeSkipped() {
		testsToPrint, _ = removeSkippedTestsFromBundle(shard.Included)
	} else {
		testsToPrint = shard.Included
	}

	// Convert driver.BundleEntity to resultsjson.Result.
	results := make([]*resultsjson.Result, len(testsToPrint))
	for i, re := range testsToPrint {
		test, err := resultsjson.NewTest(re.Resolved.GetEntity())
		if err != nil {
			return nil, err
		}
		results[i] = &resultsjson.Result{
			Test:       *test,
			SkipReason: strings.Join(re.Resolved.GetSkip().GetReasons(), ", "),
		}
	}
	return results, nil
}

// verifyTestNames returns nil if all given test names have a match.
func verifyTestNames(patterns []string, tests []*driver.BundleEntity) error {
	// Make a map of given test names (NOT patterns).
	m, err := testing.NewMatcher(patterns)
	if err != nil {
		return errors.Wrap(err, "failed parsing test patterns")
	}
	var testNames []string
	for _, t := range tests {
		testNames = append(testNames, t.Resolved.GetEntity().GetName())
	}
	unmatched := m.UnmatchedPatterns(testNames)
	if len(unmatched) != 0 {
		return errors.Errorf("no tests matched by pattern(s) %v, please try tast list to find tests with similar pattern", strings.Join(unmatched, ", "))
	}
	return nil
}

func runTests(ctx context.Context, cfg *config.Config, state *config.DeprecatedState, drv *driver.Driver, client *reporting.RPCClient, dutInfo *protocol.DUTInfo) (results []*resultsjson.Result, retErr error) {
	if err := ioutil.WriteFile(filepath.Join(cfg.ResDir(), DUTInfoFile), []byte(proto.MarshalTextString(dutInfo)), 0644); err != nil {
		logging.Debugf(ctx, "Failed to dump DUTInfo: %v", err)
	}

	if ver := dutInfo.GetOsVersion(); ver == "" {
		logging.Info(ctx, "Target version: not available from target")
	} else {
		logging.Infof(ctx, "Target version: %v", ver)
	}

	initialSysInfo, err := drv.GetSysInfoState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}

	tests, err := drv.ListMatchedTests(ctx, cfg.Features(dutInfo.GetFeatures()))
	if err != nil {
		return nil, err
	}

	if err := verifyTestNames(cfg.Patterns(), tests); err != nil {
		return nil, err
	}

	shard := sharding.Compute(tests, cfg.ShardIndex(), cfg.TotalShards())

	var testsToRun []*driver.BundleEntity
	var testsToSkip []*driver.BundleEntity
	if cfg.ExcludeSkipped() {
		testsToRun, testsToSkip = removeSkippedTestsFromBundle(shard.Included)
	} else {
		testsToRun = shard.Included
	}

	state.TestNamesToSkip = nil
	for _, t := range shard.Excluded {
		state.TestNamesToSkip = append(state.TestNamesToSkip, t.Resolved.GetEntity().GetName())
	}
	for _, t := range testsToSkip {
		state.TestNamesToSkip = append(state.TestNamesToSkip, t.Resolved.GetEntity().GetName())
	}

	if cfg.TotalShards() > 1 {
		logging.Infof(ctx, "Running shard %d/%d (tests %d/%d)", cfg.ShardIndex()+1, cfg.TotalShards(),
			len(testsToRun), len(testsToRun)+len(state.TestNamesToSkip))
	}
	if len(testsToRun) == 0 {
		// No tests to run.
		return nil, nil
	}

	// Reserve a bit of time to write results and collect system info.
	// Skip doing this if a very-short timeout was set, since it's confusing
	// to get an immediate timeout in that case.
	postCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		postReserve := maxPostReserve
		if time.Until(deadline) < 2*postReserve {
			postReserve = 0
		}
		var cancel xcontext.CancelFunc
		ctx, cancel = xcontext.WithDeadline(ctx, deadline.Add(-postReserve), errors.Errorf("%v: global timeout reached", context.DeadlineExceeded))
		defer cancel(context.Canceled)
	}

	// Write results and collect system info after testing.
	defer func() {
		ctx := postCtx
		if retErr != nil {
			// Print the run error message before moving on to writing results.
			logging.Infof(ctx, "Failed to run tests: %v", retErr)
		}

		// The DUT might have rebooted during tests. Try reconnecting
		// before proceeding to CollectSysInfo.
		if err := drv.ReconnectIfNeeded(ctx); err != nil {
			logging.Infof(ctx, "Failed to reconnect to DUT: %v", err)
		}

		// We don't want to bail out before writing test results if sysinfo
		// collection fails, but we'll still return the error later.
		if err := drv.CollectSysInfo(ctx, initialSysInfo); err != nil {
			logging.Infof(ctx, "Failed collecting system info: %v", err)
			if retErr == nil {
				retErr = errors.Wrap(err, "failed collecting system info")
			}
		}

		if err := reporting.WriteLegacyResults(filepath.Join(cfg.ResDir(), reporting.LegacyResultsFilename), results); err != nil {
			logging.Infof(ctx, "Failed writing %s: %v", reporting.LegacyResultsFilename, err)
		}

		if err := reporting.WriteJUnitXMLResults(filepath.Join(cfg.ResDir(), reporting.JUnitXMLFilename), results); err != nil {
			logging.Infof(ctx, "Failed writing %s: %v", reporting.JUnitXMLFilename, err)
		}

		complete := retErr == nil
		reporting.WriteResultsToLogs(ctx, results, cfg.ResDir(), complete)
	}()

	return drv.RunTests(ctx, shard.Included, dutInfo, client, state.RemoteDevservers)
}
