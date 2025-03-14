// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/debugger"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver"
	"go.chromium.org/tast/core/internal/minidriver/failfast"
	"go.chromium.org/tast/core/internal/minidriver/processor"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/reporting"
	"go.chromium.org/tast/core/internal/run/resultsjson"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

// TestRecursively is environment variable key which is set to "0" to disable
// recursive test running flow and support full remote fixtures.
// TODO(b/187957164): Remove this after migration has finished.
const TestRecursively = "TAST_TEST_RECURSIVELY"

// ShouldRunTestsRecursively indicates if Tast tests should be run recursively
// (i.e. remote bundle invokes local bundle).
func ShouldRunTestsRecursively() bool {
	return os.Getenv(TestRecursively) != "0"
}

// runTestsArgs holds arguments common to private methods called by RunTests.
type runTestsArgs struct {
	DUTInfo          map[string]*protocol.DUTInfo
	Counter          *failfast.Counter
	Client           *reporting.RPCClient
	RemoteDevservers []string
	SwarmingTaskID   string
	BuildBucketID    string
}

// RunTests runs specified tests per bundle.
func (d *Driver) RunTests(ctx context.Context,
	tests []*BundleEntity,
	dutInfos map[string]*protocol.DUTInfo,
	client *reporting.RPCClient,
	remoteDevservers []string,
	pushedFilesInfo []*protocol.PushedFilesInfoForDUT) ([]*resultsjson.Result, error) {
	testsPerBundle := make(map[string][]*protocol.ResolvedEntity)
	for _, t := range tests {
		testsPerBundle[t.Bundle] = append(testsPerBundle[t.Bundle], t.Resolved)
	}
	bundles := make([]string, 0, len(testsPerBundle))
	for b := range testsPerBundle {
		bundles = append(bundles, b)
	}
	sort.Strings(bundles)
	var results []*resultsjson.Result

	maxFailureCounter := failfast.NewCounter(d.cfg.MaxTestFailures())
	totalExecutionCount := d.cfg.Repeats() + 1

	if totalExecutionCount > 1 {
		logging.Infof(ctx, "Running tests repeatedly for %v times.", totalExecutionCount)
	}

	for i := 0; i < totalExecutionCount; i++ {
		for _, bundle := range bundles {
			res, err := d.runTests(ctx, bundle, testsPerBundle[bundle], dutInfos, client, remoteDevservers, pushedFilesInfo, maxFailureCounter)
			results = append(results, res...)
			if err != nil {
				return results, err
			}
		}
	}

	return results, nil
}

// runTests runs specified tests. It can return non-nil results even on errors.
func (d *Driver) runTests(ctx context.Context, bundle string,
	tests []*protocol.ResolvedEntity, dutInfos map[string]*protocol.DUTInfo,
	client *reporting.RPCClient, remoteDevservers []string,
	pushedFilesInfo []*protocol.PushedFilesInfoForDUT, maxFailureCounter *failfast.Counter) ([]*resultsjson.Result, error) {

	args := &runTestsArgs{
		DUTInfo:          dutInfos,
		Counter:          maxFailureCounter,
		Client:           client,
		RemoteDevservers: remoteDevservers,
		SwarmingTaskID:   d.cfg.SwarmingTaskID(),
		BuildBucketID:    d.cfg.BuildBucketID(),
	}

	if !ShouldRunTestsRecursively() {
		localTests, remoteTests, err := splitTests(tests)
		if err != nil {
			return nil, err
		}
		// Note: These methods can return non-nil results even on errors.
		localResults, err := d.runLocalTests(ctx, bundle, localTests, args)
		if err != nil {
			return localResults, err
		}
		var remoteTestNames []string
		for _, t := range remoteTests {
			remoteTestNames = append(remoteTestNames, t.GetEntity().GetName())
		}
		remoteResults, err := d.runRemoteTests(ctx, bundle, remoteTestNames, args, pushedFilesInfo)

		return append(localResults, remoteResults...), err
	}

	var testNames []string
	for _, t := range tests {
		testNames = append(testNames, t.GetEntity().GetName())
	}
	return d.runRemoteTests(ctx, bundle, testNames, args, pushedFilesInfo)
}

func (d *Driver) runLocalTests(ctx context.Context, bundle string, tests []*protocol.ResolvedEntity, args *runTestsArgs) ([]*resultsjson.Result, error) {
	// If there is no local test to run, return early without connecting to
	// remote test bundles.
	if len(tests) == 0 {
		return nil, nil
	}

	// We don't yet support inter-machine fixture dependencies except for
	// the primary bundle (aka "cros"). Thus
	var results []*resultsjson.Result
	testsByStart := make(map[string][]*protocol.ResolvedEntity)
	for _, t := range tests {
		start := t.GetStartFixtureName()
		if bundle == d.cfg.PrimaryBundle() || start == "" {
			testsByStart[start] = append(testsByStart[start], t)
		} else {
			// Generate a failure result immediately.
			rjTest, err := resultsjson.NewTest(t.GetEntity())
			if err != nil {
				return results, err
			}
			now := time.Now()
			results = append(results, &resultsjson.Result{
				Test: *rjTest,
				Errors: []resultsjson.Error{{
					Time:   now,
					Reason: "Local-remote fixture dependencies are not yet supported in non-primary bundles (b/187957164)",
				}},
				Start: now,
				End:   now,
			})
		}
	}

	// Sort start fixtures for deterministic execution ordering.
	starts := make([]string, 0, len(testsByStart))
	for name := range testsByStart {
		starts = append(starts, name)
	}
	sort.Strings(starts)

	for _, start := range starts {
		subresults, err := d.runLocalTestsWithRemoteFixture(ctx, bundle, testsByStart[start], start, args)
		results = append(results, subresults...)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func (d *Driver) runLocalTestsWithRemoteFixture(ctx context.Context, bundle string, tests []*protocol.ResolvedEntity, start string, args *runTestsArgs) (results []*resultsjson.Result, retErr error) {
	if start == "" {
		return d.runLocalTestsWithRetry(ctx, bundle, tests, &protocol.StartFixtureState{}, args)
	}
	runCfg, err := d.newRunFixtureConfig(args.DUTInfo[""])
	if err != nil {
		return nil, err
	}

	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)

	// Create a processor for the remote fixture. This will run in parallel
	// with the processor for local entities.
	hs := []processor.Handler{
		processor.NewLoggingHandler(d.cfg.ResDir(), multiplexer, args.Client),
		processor.NewTimingHandler(),
		processor.NewStreamedResultsHandler(d.cfg.ResDir()),
		processor.NewRPCResultsHandler(args.Client),
		processor.NewFailFastHandler(args.Counter),
		// copyOutputHandler should come last as it can block RunEnd for a while.
		processor.NewCopyOutputHandler(os.Rename),
	}
	proc := processor.New(d.cfg.ResDir(), nopDiagnose, hs, bundle)
	defer func() {
		proc.RunEnd(ctx, retErr)
	}()

	if err := proc.RunStart(ctx); err != nil {
		return nil, err
	}

	// Send EntityStart/EntityEnd events to the processor.
	if err := proc.EntityStart(ctx, &protocol.EntityStartEvent{
		Time:   timestamppb.Now(),
		OutDir: runCfg.OutDir,
		// HACK: Create a partially crafted Entity since we don't know
		// full details of the remote fixture. This should be okay since
		// Processor doesn't need fixture details.
		Entity: &protocol.Entity{
			Type: protocol.EntityType_FIXTURE,
			Name: start,
		},
	}); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			return
		}
		if err := proc.EntityEnd(ctx, &protocol.EntityEndEvent{
			Time:       timestamppb.Now(),
			EntityName: start,
			TimingLog:  nil,
		}); err != nil {
			retErr = err
		}
	}()

	cl := d.remoteBundleClient(d.cfg.PrimaryBundle())

	ticket, err := cl.RunFixture(ctx, start, runCfg, proc)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := ticket.TearDown(ctx); err != nil && retErr == nil {
			retErr = err
		}
	}()

	return d.runLocalTestsWithRetry(ctx, bundle, tests, ticket.StartFixtureState(), args)
}

func (d *Driver) runLocalTestsWithRetry(ctx context.Context, bundle string, tests []*protocol.ResolvedEntity, state *protocol.StartFixtureState, args *runTestsArgs) ([]*resultsjson.Result, error) {

	buildArtifactsURL := getBuildArtifactsURL(d.cfg.BuildArtifactsURLOverride(), args.DUTInfo[""].GetDefaultBuildArtifactsUrl())

	dutFeature := make(map[string]*frameworkprotocol.DUTFeatures)
	for key, value := range args.DUTInfo {
		dutFeature[key] = value.GetFeatures()
	}

	cfg := &minidriver.Config{
		Retries:               d.cfg.Retries(),
		ResDir:                d.cfg.ResDir(),
		Devservers:            d.cfg.Devservers(),
		Target:                d.cfg.Target(),
		LocalDataDir:          d.cfg.LocalDataDir(),
		LocalOutDir:           d.cfg.LocalOutDir(),
		LocalTempDir:          d.cfg.LocalTempDir(),
		LocalBundleDir:        d.cfg.LocalBundleDir(),
		DownloadMode:          d.cfg.DownloadMode(),
		WaitUntilReady:        d.cfg.WaitUntilReady(),
		SystemServicesTimeout: d.cfg.SystemServicesTimeout(),
		WaitUntilReadyTimeout: d.cfg.WaitUntilReadyTimeout(),
		MsgTimeout:            d.cfg.MsgTimeout(),
		CheckTestDeps:         d.cfg.CheckTestDeps(),
		TestVars:              d.cfg.TestVars(),
		MaybeMissingVars:      d.cfg.MaybeMissingVars(),
		DUTLabConfig:          d.cfg.DUTLabConfig(),
		DebuggerPort:          d.cfg.DebuggerPorts()[debugger.LocalBundle],
		Proxy:                 d.cfg.Proxy() == config.ProxyEnv,
		DUTFeatures:           dutFeature,
		ForceSkips:            d.cfg.ForceSkips(),
		Factory:               minidriver.NewRootHandlersFactory(d.cfg.ResDir(), args.Counter, args.Client),
		BuildArtifactsURL:     buildArtifactsURL,
		SwarmingTaskID:        d.cfg.SwarmingTaskID(),
		BuildBucketID:         d.cfg.BuildBucketID(),
	}
	md := minidriver.NewDriver(cfg, d.cc)
	var names []string
	for _, t := range tests {
		names = append(names, t.GetEntity().GetName())
	}
	return md.RunLocalTests(ctx, bundle, names, state)
}

func (d *Driver) runRemoteTests(ctx context.Context, bundle string, tests []string,
	args *runTestsArgs, pushedFilesInfo []*protocol.PushedFilesInfoForDUT) ([]*resultsjson.Result, error) {
	if len(tests) == 0 {
		return nil, nil
	}

	runTestsOnce := func(ctx context.Context, tests []string) ([]*resultsjson.Result, error) {
		return d.runRemoteTestsOnce(ctx, bundle, tests, args, pushedFilesInfo)
	}
	return minidriver.RunTestsWithRetry(ctx, tests, runTestsOnce, d.cfg.Retries())
}

func (d *Driver) runRemoteTestsOnce(ctx context.Context, bundle string, tests []string, args *runTestsArgs,
	pushedFilesInfo []*protocol.PushedFilesInfoForDUT) ([]*resultsjson.Result, error) {
	bcfg, rcfg, err := d.newConfigsForRemoteTests(ctx, tests, args.DUTInfo, args.RemoteDevservers,
		args.SwarmingTaskID, args.BuildBucketID, pushedFilesInfo)
	if err != nil {
		return nil, err
	}
	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)

	hs := []processor.Handler{
		processor.NewLoggingHandler(d.cfg.ResDir(), multiplexer, args.Client),
		processor.NewTimingHandler(),
		processor.NewStreamedResultsHandler(d.cfg.ResDir()),
		processor.NewRPCResultsHandler(args.Client),
		processor.NewFailFastHandler(args.Counter),
		// copyOutputHandler should come last as it can block RunEnd for a while.
		processor.NewCopyOutputHandler(os.Rename),
	}
	proc := processor.New(d.cfg.ResDir(), nopDiagnose, hs, bundle)
	d.remoteBundleClient(bundle).RunTests(ctx, bcfg, rcfg, proc, ShouldRunTestsRecursively())
	return proc.Results(), proc.FatalError()
}

func (d *Driver) newConfigsForRemoteTests(ctx context.Context, tests []string,
	dutInfos map[string]*protocol.DUTInfo,
	remoteDevservers []string, swarmingTaskID,
	buildBucketID string,
	pushedFilesInfo []*protocol.PushedFilesInfoForDUT) (*protocol.BundleConfig, *protocol.RunConfig, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}

	companionDUTs := make(map[string]*protocol.DUTConfig)
	for name, target := range d.cfg.CompanionDUTs() {
		// Don't add to the map if we don't have a valid target.
		if target == "" || target == "-" {
			continue
		}

		resolvedTarget, resolvedProxyCommand := resolveSSHConfig(ctx, target)
		proxyCommand := d.cfg.ProxyCommand()
		if proxyCommand == "" {
			proxyCommand = resolvedProxyCommand
		}

		companionDUTs[name] = &protocol.DUTConfig{
			SshConfig: &protocol.SSHConfig{
				// TODO: Resolve target to a connection spec.
				ConnectionSpec: resolvedTarget,
				KeyFile:        d.cfg.KeyFile(),
				KeyDir:         d.cfg.KeyDir(),
				ProxyCommand:   proxyCommand,
			},
			TlwName: target,
		}
	}

	buildArtifactsURL := d.cfg.BuildArtifactsURLOverride()
	if buildArtifactsURL == "" {
		buildArtifactsURL = dutInfos[""].GetDefaultBuildArtifactsUrl()
	}

	var connSpec, proxyCommand string
	var primaryTarget *protocol.TargetDevice
	if d.cc != nil {
		connSpec = d.cc.ConnectionSpec()
		proxyCommand = d.cc.ProxyCommand()
	}
	if connSpec != "" && connSpec != "-" {
		primaryTarget = &protocol.TargetDevice{
			DutConfig: &protocol.DUTConfig{
				SshConfig: &protocol.SSHConfig{
					ConnectionSpec: connSpec,
					KeyFile:        d.cfg.KeyFile(),
					KeyDir:         d.cfg.KeyDir(),
					ProxyCommand:   proxyCommand,
				},
				TlwName: d.cfg.Target(),
			},
			BundleDir: d.cfg.LocalBundleDir(),
		}
	}

	bcfg := &protocol.BundleConfig{
		PrimaryTarget: primaryTarget,
		CompanionDuts: companionDUTs,
		MetaTestConfig: &protocol.MetaTestConfig{
			TastPath:  exe,
			RunFlags:  d.runFlags(buildArtifactsURL),
			ListFlags: d.listFlags(buildArtifactsURL),
		},
	}
	CompanionFeatures := make(map[string]*frameworkprotocol.DUTFeatures)
	for role, dutInfo := range dutInfos {
		if role != "" {
			CompanionFeatures[role] = dutInfo.GetFeatures()
		}
	}
	rcfg := &protocol.RunConfig{
		Tests: tests,
		Dirs: &protocol.RunDirectories{
			DataDir: d.cfg.RemoteDataDir(),
			OutDir:  d.cfg.RemoteOutDir(),
			TempDir: d.cfg.RemoteTempDir(),
		},
		Features: d.cfg.Features(dutInfos[""].GetFeatures(), CompanionFeatures),
		ServiceConfig: &protocol.ServiceConfig{
			Devservers: remoteDevservers,
			TlwServer:  d.cfg.TLWServer(),
			// DutServer is intentally left blank here because this config is only used by
			// remote fixture which will use devserver or ephemeral server to download files.

			UseEphemeralDevservers: d.cfg.UseEphemeralDevserver(),
			TastDir:                d.cfg.TastDir(),
			ExtraAllowedBuckets:    d.cfg.ExtraAllowedBuckets(),
			SwarmingTaskID:         swarmingTaskID,
			BuildBucketID:          buildBucketID,
		},
		DataFileConfig: &protocol.DataFileConfig{
			DownloadMode:      d.cfg.DownloadMode(),
			BuildArtifactsUrl: buildArtifactsURL,
		},
		HeartbeatInterval:     durationpb.New(minidriver.HeartbeatInterval),
		DebugPort:             uint32(d.cfg.DebuggerPorts()[debugger.RemoteBundle]),
		SystemServicesTimeout: durationpb.New(d.cfg.SystemServicesTimeout()),
		WaitUntilReadyTimeout: durationpb.New(d.cfg.WaitUntilReadyTimeout()),
		MsgTimeout:            durationpb.New(d.cfg.MsgTimeout()),
		MaxSysMsgLogSize:      d.cfg.MaxSysMsgLogSize(),
		PushedFilesInfo:       pushedFilesInfo,
		Target: &protocol.RunTargetConfig{
			Devservers: d.cfg.Devservers(),
			Dirs: &protocol.RunDirectories{
				DataDir: d.cfg.LocalDataDir(),
				OutDir:  d.cfg.LocalOutDir(),
				TempDir: d.cfg.LocalTempDir(),
			},
			DebugPort:             uint32(d.cfg.DebuggerPorts()[debugger.LocalBundle]),
			MaxTestFailures:       int32(d.cfg.MaxTestFailures()),
			Retries:               int32(d.cfg.Retries()),
			Proxy:                 d.cfg.Proxy() == config.ProxyEnv,
			WaitUntilReady:        d.cfg.WaitUntilReady(),
			MsgTimeout:            durationpb.New(d.cfg.MsgTimeout()),
			SystemServicesTimeout: durationpb.New(d.cfg.SystemServicesTimeout()),
			WaitUntilReadyTimeout: durationpb.New(d.cfg.WaitUntilReadyTimeout()),
			SwarmingTaskID:        d.cfg.SwarmingTaskID(),
			BuildBucketID:         d.cfg.BuildBucketID(),
		},
	}
	return bcfg, rcfg, nil
}

func (d *Driver) runFlags(buildArtifactsURL string) []string {
	runFlags := []string{
		"-build=" + strconv.FormatBool(d.cfg.Build()),
		"-keyfile=" + d.cfg.KeyFile(),
		"-keydir=" + d.cfg.KeyDir(),
		"-remoterunner=" + d.cfg.RemoteRunner(),
		"-remotebundledir=" + d.cfg.RemoteBundleDir(),
		"-remotedatadir=" + d.cfg.RemoteDataDir(),
		"-localrunner=" + d.cfg.LocalRunner(),
		"-localbundledir=" + d.cfg.LocalBundleDir(),
		"-localdatadir=" + d.cfg.LocalDataDir(),
		"-devservers=" + strings.Join(d.cfg.Devservers(), ","),
		"-buildartifactsurl=" + buildArtifactsURL,
	}
	for _, varsDir := range d.cfg.DefaultVarsDirs() {
		runFlags = append(runFlags, "-defaultvarsdir="+varsDir)
	}
	for role, dut := range d.cfg.CompanionDUTs() {
		runFlags = append(runFlags, fmt.Sprintf("-companiondut=%s:%s", role, dut))
	}
	return runFlags
}

func (d *Driver) listFlags(buildArtifactsURL string) []string {
	runFlags := []string{
		"-build=" + strconv.FormatBool(d.cfg.Build()),
		"-keyfile=" + d.cfg.KeyFile(),
		"-keydir=" + d.cfg.KeyDir(),
		"-remoterunner=" + d.cfg.RemoteRunner(),
		"-remotebundledir=" + d.cfg.RemoteBundleDir(),
		"-remotedatadir=" + d.cfg.RemoteDataDir(),
		"-localrunner=" + d.cfg.LocalRunner(),
		"-localbundledir=" + d.cfg.LocalBundleDir(),
		"-localdatadir=" + d.cfg.LocalDataDir(),
		"-devservers=" + strings.Join(d.cfg.Devservers(), ","),
		"-buildartifactsurl=" + buildArtifactsURL,
	}
	return runFlags
}

func getBuildArtifactsURL(buildArtifactsURLOverride, dutDefaultBuildArtifactsURL string) string {

	//If the tast cmd does not provide buildArtifactsUrl then use the dut default build artifact url
	if buildArtifactsURLOverride == "" {
		return dutDefaultBuildArtifactsURL
	}
	return buildArtifactsURLOverride
}

func (d *Driver) newRunFixtureConfig(dutInfo *protocol.DUTInfo) (*protocol.RunFixtureConfig, error) {
	var tlwServer string
	if addr, ok := d.cc.Conn().Services().TLWAddr(); ok {
		tlwServer = addr.String()
	}

	buildArtifactsURL := getBuildArtifactsURL(d.cfg.BuildArtifactsURLOverride(), dutInfo.GetDefaultBuildArtifactsUrl())
	var connSpec, proxyCommand string
	if d.cc != nil {
		connSpec = d.cc.ConnectionSpec()
		proxyCommand = d.cc.ProxyCommand()
	}
	return &protocol.RunFixtureConfig{
		TestVars:          d.cfg.TestVars(),
		DataDir:           d.cfg.RemoteDataDir(),
		OutDir:            d.cfg.RemoteOutDir(),
		TempDir:           "", // empty for fixture service to create it
		ConnectionSpec:    connSpec,
		KeyFile:           d.cfg.KeyFile(),
		KeyDir:            d.cfg.KeyDir(),
		LocalBundleDir:    d.cfg.LocalBundleDir(),
		CheckSoftwareDeps: false,
		Devservers:        d.cfg.Devservers(),
		// DutServer is intentally left blank here because this config is only used by
		// remote fixture which will use devserver or ephemeral server to download files.
		TlwServer:         tlwServer,
		DutName:           d.cfg.Target(),
		BuildArtifactsUrl: buildArtifactsURL,
		DownloadMode:      d.cfg.DownloadMode(),
		ProxyCommand:      proxyCommand,
	}, nil
}

func splitTests(tests []*protocol.ResolvedEntity) (localTests, remoteTests []*protocol.ResolvedEntity, err error) {
	for _, t := range tests {
		switch t.GetHops() {
		case 0:
			remoteTests = append(remoteTests, t)
		case 1:
			localTests = append(localTests, t)
		default:
			return nil, nil, errors.Errorf("unsupported hop %d for test %s", t.GetHops(), t.GetEntity().GetName())
		}
	}
	return localTests, remoteTests, nil
}

// nopDiagnose is a DiagnoseFunc that does nothing.
func nopDiagnose(ctx context.Context, outDir string) string {
	return ""
}
