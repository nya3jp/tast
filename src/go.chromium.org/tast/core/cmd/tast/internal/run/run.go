// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/cmd/tast/internal/run/prepare"
	"go.chromium.org/tast/core/cmd/tast/internal/run/sharding"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/devserver"
	"go.chromium.org/tast/core/internal/run/reporting"
	"go.chromium.org/tast/core/internal/run/resultsjson"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/internal/xcontext"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
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
	if !config.ShouldConnect(cfg.Target()) {
		logging.Info(ctx, "Tast will not make any connection to the target '-'.")
	}

	reportClient, err := reporting.NewRPCClient(ctx, cfg.ReportsServer())
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up gRPC servers")
	}
	defer reportClient.Close()

	state.RemoteDevservers = cfg.Devservers()
	// Always start an ephemeral devserver for remote tests if TLWServer is not specified, and allowed.
	if cfg.TLWServer() == "" && cfg.UseEphemeralDevserver() && config.ShouldConnect(cfg.Target()) {
		es, esAddr, err := startEphemeralDevserverForRemoteTests(ctx, cfg, state)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start ephemeral devserver for remote tests")
		}
		state.RemoteDevservers = append([]string{esAddr}, state.RemoteDevservers...)
		defer es.Close()
	}

	if err := prepare.CheckPrivateBundleFlag(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed in checking downloadprivatebundles flag")
	}
	drv, err := driver.New(ctx, cfg, cfg.Target(), "", state.RemoteDevservers)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to target")
	}
	defer drv.Close(ctx)
	dutInfo, pushedFilesInfo, err := prepareEnv(ctx, cfg, drv)
	if err != nil {
		return nil, err
	}

	switch cfg.Mode() {
	case config.ListTestsMode:
		results, err := listTests(ctx, cfg, drv, dutInfo)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list tests")
		}
		return results, nil
	case config.RunTestsMode:
		results, err := runTests(ctx, cfg, state, drv, reportClient, dutInfo, pushedFilesInfo)
		if err != nil {
			return results, errors.Wrapf(err, "failed to run tests")
		}
		return results, nil
	default:
		return nil, errors.Errorf("unhandled mode %d", cfg.Mode())
	}
}

func prepareEnv(ctx context.Context, cfg *config.Config, drv *driver.Driver) (
	map[string]*protocol.DUTInfo, []*protocol.PushedFilesInfoForDUT, error) {
	var pushedFilesInfo []*protocol.PushedFilesInfoForDUT
	dutInfo := make(map[string]*protocol.DUTInfo)
	if err := prepare.SetUpRemotePrivateBundle(ctx, cfg, drv); err != nil {
		return nil, nil, errors.Wrap(err, "failed to prepare Host")
	}
	primaryDutInfo, pushedExecutables, err := prepare.Prepare(ctx, cfg, drv)
	dutInfo[""] = primaryDutInfo
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to build and push primary DUT")
	}
	pushedFilesInfo = append(pushedFilesInfo, &protocol.PushedFilesInfoForDUT{Role: "", SrcDstPaths: pushedExecutables})

	for role, dut := range cfg.CompanionDUTs() {
		companionDriver, err := driver.New(ctx, cfg, dut, role, nil)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to connect to companion DUT %s", dut)
		}
		defer companionDriver.Close(ctx)
		dutInfo[role], pushedExecutables, err = prepare.Prepare(ctx, cfg, companionDriver)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to build and push companion DUT %s", dut)
		}
		pushedFilesInfo = append(pushedFilesInfo, &protocol.PushedFilesInfoForDUT{Role: role, SrcDstPaths: pushedExecutables})
	}

	return dutInfo, pushedFilesInfo, nil
}

// startEphemeralDevserverForRemoteTests starts an ephemeral devserver for remote tests.
func startEphemeralDevserverForRemoteTests(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) (*devserver.Ephemeral, string, error) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, "", fmt.Errorf("failed to listen to a local port: %v", err)
	}

	cacheDir := filepath.Join(cfg.TastDir(), "devserver", "static")
	es, err := devserver.NewEphemeral(lis, cacheDir, cfg.ExtraAllowedBuckets())
	if err != nil {
		return nil, "", err
	}

	addr := fmt.Sprintf("http://%s", lis.Addr())
	logging.Info(ctx, "Starting ephemeral devserver at ", addr, " for remote tests")
	return es, addr, nil
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

// GlobalRuntimeVars returns all used global runtime variables.
func GlobalRuntimeVars(ctx context.Context, cfg *config.Config, state *config.DeprecatedState) ([]string, error) {

	if err := prepare.CheckPrivateBundleFlag(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed in checking downloadprivatebundles flag")
	}

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", cfg.Devservers())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to target")
	}
	defer drv.Close(ctx)
	_, _, err = prepareEnv(ctx, cfg, drv)

	if err != nil {
		return nil, err
	}

	vars, err := drv.GlobalRuntimeVars(ctx)
	if err != nil {
		return nil, err
	}
	return vars, err
}

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *config.Config,
	drv *driver.Driver,
	dutInfos map[string]*protocol.DUTInfo) ([]*resultsjson.Result, error) {
	CompanionFeatures := make(map[string]*frameworkprotocol.DUTFeatures)
	for role, dutInfo := range dutInfos {
		if role != "" {
			CompanionFeatures[role] = dutInfo.GetFeatures()
		}
	}

	var dutFeature *frameworkprotocol.DUTFeatures
	if _, ok := dutInfos[""]; ok {
		dutFeature = dutInfos[""].GetFeatures()
	}

	tests, err := drv.ListMatchedTests(ctx, cfg.Features(dutFeature, CompanionFeatures))
	if err != nil {
		return nil, err
	}

	var shard *sharding.Shard
	if cfg.ShardMethod() == "hash" {
		shard = sharding.ComputeHash(tests, cfg.ShardIndex(), cfg.TotalShards())
	} else {
		shard = sharding.ComputeAlpha(tests, cfg.ShardIndex(), cfg.TotalShards())
	}

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

func runTests(ctx context.Context, cfg *config.Config,
	state *config.DeprecatedState,
	drv *driver.Driver, client *reporting.RPCClient,
	dutInfos map[string]*protocol.DUTInfo,
	pushedFilesInfo []*protocol.PushedFilesInfoForDUT) (results []*resultsjson.Result,
	retErr error) {

	var roles []string
	for role := range dutInfos {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		dir := cfg.ResDir()
		roleName := "Primary"
		if role != "" {
			dir = filepath.Join(cfg.ResDir(), role)
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				logging.Debugf(ctx, "Failed to create directory: %v", err)
			}
			roleName = role
		}
		if err := os.WriteFile(filepath.Join(dir, DUTInfoFile), []byte(prototext.Format(dutInfos[role])), 0644); err != nil {
			logging.Debugf(ctx, "Failed to dump DUTInfo: %v", err)
		}

		if ver := dutInfos[role].GetOsVersion(); ver == "" {
			logging.Infof(ctx, "%s DUT version: not available from target", roleName)
		} else {
			logging.Infof(ctx, "%s DUT version: %v", roleName, ver)
		}
	}

	initialSysInfo, err := drv.GetSysInfoState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}

	postCtx := ctx
	systemLogsSaved := false
	collectSystemLog := func(ctx context.Context) {
		if systemLogsSaved {
			return
		}
		// The DUT might have rebooted during tests. Try reconnecting
		// before proceeding to CollectSysInfo.
		if err := drv.ReconnectIfNeeded(ctx, true, false); err != nil {
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
		systemLogsSaved = true
		logging.Info(ctx, "Done collecting system logs")
	}
	defer collectSystemLog(postCtx)

	CompanionFeatures := make(map[string]*frameworkprotocol.DUTFeatures)
	for role, dutInfo := range dutInfos {
		if role != "" {
			CompanionFeatures[role] = dutInfo.GetFeatures()
		}
	}

	tests, err := drv.ListMatchedTests(ctx, cfg.Features(dutInfos[""].GetFeatures(), CompanionFeatures))
	if err != nil {
		return nil, err
	}

	if err := verifyTestNames(cfg.Patterns(), tests); err != nil {
		return nil, err
	}

	var shard *sharding.Shard
	if cfg.ShardMethod() == "hash" {
		shard = sharding.ComputeHash(tests, cfg.ShardIndex(), cfg.TotalShards())
	} else {
		shard = sharding.ComputeAlpha(tests, cfg.ShardIndex(), cfg.TotalShards())
	}

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
	if deadline, ok := ctx.Deadline(); ok {
		postReserve := maxPostReserve
		if time.Until(deadline) < 2*postReserve {
			postReserve = 0
		}
		var cancel xcontext.CancelFunc
		ctx, cancel = xcontext.WithDeadline(ctx, deadline.Add(-postReserve), errors.Errorf("%v: tast command timeout reached", context.DeadlineExceeded))
		defer cancel(context.Canceled)
	}

	// Write results and collect system info after testing.
	defer func() {
		cmdTimeoutPast := ctxutil.DeadlineBefore(ctx, time.Now())

		ctx := postCtx
		if retErr != nil {
			// Print the run error message before moving on to writing results.
			logging.Infof(ctx, "Failed to run tests: %v", retErr)
		}

		collectSystemLog(ctx)

		if err := reporting.WriteLegacyResults(filepath.Join(cfg.ResDir(), reporting.LegacyResultsFilename), results); err != nil {
			logging.Infof(ctx, "Failed writing %s: %v", reporting.LegacyResultsFilename, err)
		}

		if err := reporting.WriteJUnitXMLResults(filepath.Join(cfg.ResDir(), reporting.JUnitXMLFilename), results); err != nil {
			logging.Infof(ctx, "Failed writing %s: %v", reporting.JUnitXMLFilename, err)
		}

		if err := drv.CollectServoLogs(ctx); err != nil {
			logging.Infof(ctx, "Failed writing servod logs: %v", err)
		}

		complete := retErr == nil

		logging.Info(ctx, "Done collecting logs")

		reporting.WriteResultsToLogs(ctx, results, cfg.ResDir(), complete, cmdTimeoutPast)
	}()

	return drv.RunTests(ctx, shard.Included, dutInfos, client, state.RemoteDevservers, pushedFilesInfo)
}
