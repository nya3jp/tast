// Copyright 2021 The Chromium OS Authors. All rights reserved.
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

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver"
	"chromiumos/tast/internal/minidriver/failfast"
	"chromiumos/tast/internal/minidriver/processor"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/reporting"
	"chromiumos/tast/internal/run/resultsjson"
)

// runTestsArgs holds arguments common to private methods called by RunTests.
type runTestsArgs struct {
	DUTInfo          *protocol.DUTInfo
	Counter          *failfast.Counter
	Client           *reporting.RPCClient
	RemoteDevservers []string
}

// RunTests runs specified tests per bundle.
func (d *Driver) RunTests(ctx context.Context, tests []*BundleEntity, dutInfo *protocol.DUTInfo, client *reporting.RPCClient, remoteDevservers []string) ([]*resultsjson.Result, error) {
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
	for _, bundle := range bundles {
		res, err := d.runTests(ctx, bundle, testsPerBundle[bundle], dutInfo, client, remoteDevservers)
		results = append(results, res...)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// runTests runs specified tests. It can return non-nil results even on errors.
func (d *Driver) runTests(ctx context.Context, bundle string, tests []*protocol.ResolvedEntity, dutInfo *protocol.DUTInfo, client *reporting.RPCClient, remoteDevservers []string) ([]*resultsjson.Result, error) {
	localTests, remoteTests, err := splitTests(tests)
	if err != nil {
		return nil, err
	}

	args := &runTestsArgs{
		DUTInfo:          dutInfo,
		Counter:          failfast.NewCounter(d.cfg.MaxTestFailures()),
		Client:           client,
		RemoteDevservers: remoteDevservers,
	}

	// Note: These methods can return non-nil results even on errors.
	localResults, err := d.runLocalTests(ctx, bundle, localTests, args)
	if err != nil {
		return localResults, err
	}
	remoteResults, err := d.runRemoteTests(ctx, bundle, remoteTests, args)
	return append(localResults, remoteResults...), err
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
	runCfg, err := d.newRunFixtureConfig(args.DUTInfo)
	if err != nil {
		return nil, err
	}

	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)

	// Create a processor for the remote fixture. This will run in parallel
	// with the processor for local entities.
	hs := processor.NewHandlers(d.cfg.ResDir(), multiplexer, nopDiagnose, os.Rename, nil, args.Client)
	proc := processor.New(d.cfg.ResDir(), nopDiagnose, hs)
	defer func() {
		proc.RunEnd(ctx, retErr)
	}()

	if err := proc.RunStart(ctx); err != nil {
		return nil, err
	}

	// Send EntityStart/EntityEnd events to the processor.
	if err := proc.EntityStart(ctx, &protocol.EntityStartEvent{
		Time:   ptypes.TimestampNow(),
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
			Time:       ptypes.TimestampNow(),
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
	_, useDebugger := d.cfg.DebuggerPorts()[debugger.LocalBundle]
	cfg := &minidriver.Config{
		Retries:          d.cfg.Retries(),
		ResDir:           d.cfg.ResDir(),
		Devservers:       d.cfg.Devservers(),
		Target:           d.cfg.Target(),
		LocalDataDir:     d.cfg.LocalDataDir(),
		LocalOutDir:      d.cfg.LocalOutDir(),
		LocalTempDir:     d.cfg.LocalTempDir(),
		LocalBundleDir:   d.cfg.LocalBundleDir(),
		DownloadMode:     d.cfg.DownloadMode(),
		WaitUntilReady:   d.cfg.WaitUntilReady(),
		CheckTestDeps:    d.cfg.CheckTestDeps(),
		TestVars:         d.cfg.TestVars(),
		MaybeMissingVars: d.cfg.MaybeMissingVars(),
		UseDebugger:      useDebugger,
		Proxy:            d.cfg.Proxy() == config.ProxyEnv,
		DUTFeatures:      args.DUTInfo.GetFeatures(),
		Counter:          args.Counter,
		Client:           args.Client,
	}
	md := minidriver.NewDriver(cfg, d.cc)
	return md.RunLocalTests(ctx, bundle, tests, state)
}

func (d *Driver) runRemoteTests(ctx context.Context, bundle string, tests []*protocol.ResolvedEntity, args *runTestsArgs) ([]*resultsjson.Result, error) {
	if len(tests) == 0 {
		return nil, nil
	}

	runTestsOnce := func(ctx context.Context, tests []*protocol.ResolvedEntity) ([]*resultsjson.Result, error) {
		return d.runRemoteTestsOnce(ctx, bundle, tests, args)
	}
	return minidriver.RunTestsWithRetry(ctx, tests, runTestsOnce, d.cfg.Retries())
}

func (d *Driver) runRemoteTestsOnce(ctx context.Context, bundle string, tests []*protocol.ResolvedEntity, args *runTestsArgs) ([]*resultsjson.Result, error) {
	bcfg, rcfg, err := d.newConfigsForRemoteTests(tests, args.DUTInfo, args.RemoteDevservers)
	if err != nil {
		return nil, err
	}
	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)

	hs := processor.NewHandlers(d.cfg.ResDir(), multiplexer, nopDiagnose, os.Rename, args.Counter, args.Client)
	proc := processor.New(d.cfg.ResDir(), nopDiagnose, hs)
	d.remoteBundleClient(bundle).RunTests(ctx, bcfg, rcfg, proc)
	return proc.Results(), proc.FatalError()
}

func (d *Driver) newConfigsForRemoteTests(tests []*protocol.ResolvedEntity, dutInfo *protocol.DUTInfo, remoteDevservers []string) (*protocol.BundleConfig, *protocol.RunConfig, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}

	var testNames []string
	for _, t := range tests {
		testNames = append(testNames, t.GetEntity().GetName())
	}

	companionDUTs := make(map[string]*protocol.DUTConfig)
	for name, target := range d.cfg.CompanionDUTs() {
		companionDUTs[name] = &protocol.DUTConfig{
			SshConfig: &protocol.SSHConfig{
				// TODO: Resolve target to a connection spec.
				ConnectionSpec: target,
				KeyFile:        d.cfg.KeyFile(),
				KeyDir:         d.cfg.KeyDir(),
			},
			TlwName: target,
		}
	}

	buildArtifactsURL := d.cfg.BuildArtifactsURLOverride()
	if buildArtifactsURL == "" {
		buildArtifactsURL = dutInfo.GetDefaultBuildArtifactsUrl()
	}

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
	for role, dut := range d.cfg.CompanionDUTs() {
		runFlags = append(runFlags, fmt.Sprintf("-companiondut=%s:%s", role, dut))
	}

	bcfg := &protocol.BundleConfig{
		PrimaryTarget: &protocol.TargetDevice{
			DutConfig: &protocol.DUTConfig{
				SshConfig: &protocol.SSHConfig{
					ConnectionSpec: d.cc.ConnectionSpec(),
					KeyFile:        d.cfg.KeyFile(),
					KeyDir:         d.cfg.KeyDir(),
				},
				TlwName: d.cfg.Target(),
			},
			BundleDir: d.cfg.LocalBundleDir(),
		},
		CompanionDuts: companionDUTs,
		MetaTestConfig: &protocol.MetaTestConfig{
			TastPath: exe,
			RunFlags: runFlags,
		},
	}
	rcfg := &protocol.RunConfig{
		Tests: testNames,
		Dirs: &protocol.RunDirectories{
			DataDir: d.cfg.RemoteDataDir(),
			OutDir:  d.cfg.RemoteOutDir(),
			TempDir: d.cfg.RemoteTempDir(),
		},
		Features: d.cfg.Features(dutInfo.GetFeatures()),
		ServiceConfig: &protocol.ServiceConfig{
			Devservers: remoteDevservers,
			TlwServer:  d.cfg.TLWServer(),
			// DutServer is intentally left blank here because this config is only used by
			// remote fixture which will use devserver or ephemeral server to download files.
		},
		DataFileConfig: &protocol.DataFileConfig{
			DownloadMode:      d.cfg.DownloadMode().Proto(),
			BuildArtifactsUrl: buildArtifactsURL,
		},
		HeartbeatInterval: ptypes.DurationProto(minidriver.HeartbeatInterval),
		DebugPort:         uint32(d.cfg.DebuggerPorts()[debugger.RemoteBundle]),
	}
	return bcfg, rcfg, nil
}

func (d *Driver) newRunFixtureConfig(dutInfo *protocol.DUTInfo) (*bundle.RunFixtureConfig, error) {
	var tlwServer string
	if addr, ok := d.cc.Conn().Services().TLWAddr(); ok {
		tlwServer = addr.String()
	}

	buildArtifactsURL := d.cfg.BuildArtifactsURLOverride()
	if buildArtifactsURL == "" {
		buildArtifactsURL = dutInfo.GetDefaultBuildArtifactsUrl()
	}

	var dm bundle.RunFixtureConfig_PlannerDownloadMode
	switch d.cfg.DownloadMode() {
	case planner.DownloadBatch:
		dm = bundle.RunFixtureConfig_BATCH
	case planner.DownloadLazy:
		dm = bundle.RunFixtureConfig_LAZY
	default:
		return nil, errors.Errorf("unknown mode %v", d.cfg.DownloadMode())
	}

	return &bundle.RunFixtureConfig{
		TestVars:          d.cfg.TestVars(),
		DataDir:           d.cfg.RemoteDataDir(),
		OutDir:            d.cfg.RemoteOutDir(),
		TempDir:           "", // empty for fixture service to create it
		ConnectionSpec:    d.cc.ConnectionSpec(),
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
		DownloadMode:      dm,
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
