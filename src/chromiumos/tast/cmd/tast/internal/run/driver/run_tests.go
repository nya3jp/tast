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

	"chromiumos/tast/cmd/tast/internal/run/diagnose"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/failfast"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/processor"
	"chromiumos/tast/cmd/tast/internal/run/reporting"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
)

const (
	heartbeatInterval = time.Second // interval for heartbeat messages
)

// runTestsArgs holds arguments common to private methods called by RunTests.
type runTestsArgs struct {
	DUTInfo          *protocol.DUTInfo
	Counter          *failfast.Counter
	Client           *reporting.RPCClient
	RemoteDevservers []string
}

// RunTests runs specified tests.
func (d *Driver) RunTests(ctx context.Context, tests []*BundleEntity, dutInfo *protocol.DUTInfo, client *reporting.RPCClient, remoteDevservers []string) ([]*resultsjson.Result, error) {
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
	localResults, err := d.runLocalTests(ctx, localTests, args)
	if err != nil {
		return localResults, err
	}
	remoteResults, err := d.runRemoteTests(ctx, remoteTests, args)
	return append(localResults, remoteResults...), err
}

func (d *Driver) runLocalTests(ctx context.Context, tests []*BundleEntity, args *runTestsArgs) ([]*resultsjson.Result, error) {
	// If there is no local test to run, return early without connecting to
	// remote test bundles.
	if len(tests) == 0 {
		return nil, nil
	}

	// We don't yet support inter-machine fixture dependencies except for
	// the primary bundle (aka "cros"). Thus
	var results []*resultsjson.Result
	testsByStart := make(map[string][]*BundleEntity)
	for _, t := range tests {
		start := t.Resolved.GetStartFixtureName()
		if t.Bundle == d.cfg.PrimaryBundle() || start == "" {
			testsByStart[start] = append(testsByStart[start], t)
		} else {
			// Generate a failure result immediately.
			rjTest, err := resultsjson.NewTest(t.Resolved.GetEntity())
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
		subresults, err := d.runLocalTestsWithRemoteFixture(ctx, testsByStart[start], start, args)
		results = append(results, subresults...)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func (d *Driver) runLocalTestsWithRemoteFixture(ctx context.Context, tests []*BundleEntity, start string, args *runTestsArgs) (results []*resultsjson.Result, retErr error) {
	if start == "" {
		return d.runLocalTestsWithRetry(ctx, tests, &protocol.StartFixtureState{}, args)
	}
	runCfg, err := d.newRunFixtureConfig()
	if err != nil {
		return nil, err
	}

	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)

	// Create a processor for the remote fixture. This will run in parallel
	// with the processor for local entities.
	proc := processor.New(d.cfg.ResDir(), multiplexer, nopDiagnose, os.Rename, nil, args.Client)
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

	return d.runLocalTestsWithRetry(ctx, tests, ticket.StartFixtureState(), args)
}

func (d *Driver) runLocalTestsWithRetry(ctx context.Context, tests []*BundleEntity, state *protocol.StartFixtureState, args *runTestsArgs) ([]*resultsjson.Result, error) {
	runTestsOnce := func(ctx context.Context, tests []*BundleEntity) ([]*resultsjson.Result, error) {
		return d.runLocalTestsOnce(ctx, tests, state, args)
	}
	return runTestsWithRetry(ctx, tests, runTestsOnce, d.cfg.Retries())
}

func (d *Driver) runLocalTestsOnce(ctx context.Context, tests []*BundleEntity, state *protocol.StartFixtureState, args *runTestsArgs) ([]*resultsjson.Result, error) {
	if err := d.ReconnectIfNeeded(ctx); err != nil {
		return nil, err
	}

	bcfg, rcfg := d.newConfigsForLocalTests(tests, state, args.DUTInfo)
	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)
	diag := func(ctx context.Context, outDir string) string {
		if ctxutil.DeadlineBefore(ctx, time.Now().Add(SSHPingTimeout)) {
			return ""
		}
		if err := d.SSHConn().Ping(ctx, SSHPingTimeout); err == nil {
			return ""
		}
		return "Lost SSH connection: " + diagnose.SSHDrop(ctx, d, outDir)
	}
	pull := func(src, dst string) error {
		return linuxssh.GetAndDeleteFile(ctx, d.cc.Conn().SSHConn(), src, dst, linuxssh.PreserveSymlinks)
	}

	proc := processor.New(d.cfg.ResDir(), multiplexer, diag, pull, args.Counter, args.Client)
	d.localClient().RunTests(ctx, bcfg, rcfg, proc)
	return proc.Results(), proc.FatalError()
}

func (d *Driver) runRemoteTests(ctx context.Context, tests []*BundleEntity, args *runTestsArgs) ([]*resultsjson.Result, error) {
	if len(tests) == 0 {
		return nil, nil
	}

	runTestsOnce := func(ctx context.Context, tests []*BundleEntity) ([]*resultsjson.Result, error) {
		return d.runRemoteTestsOnce(ctx, tests, args)
	}
	return runTestsWithRetry(ctx, tests, runTestsOnce, d.cfg.Retries())
}

func (d *Driver) runRemoteTestsOnce(ctx context.Context, tests []*BundleEntity, args *runTestsArgs) ([]*resultsjson.Result, error) {
	bcfg, rcfg, err := d.newConfigsForRemoteTests(tests, args.DUTInfo, args.RemoteDevservers)
	if err != nil {
		return nil, err
	}
	multiplexer := logging.NewMultiLogger()
	ctx = logging.AttachLogger(ctx, multiplexer)

	proc := processor.New(d.cfg.ResDir(), multiplexer, nopDiagnose, os.Rename, args.Counter, args.Client)
	d.remoteClient().RunTests(ctx, bcfg, rcfg, proc)
	return proc.Results(), proc.FatalError()
}

func (d *Driver) newConfigsForLocalTests(tests []*BundleEntity, state *protocol.StartFixtureState, dutInfo *protocol.DUTInfo) (*protocol.BundleConfig, *protocol.RunConfig) {
	var testNames []string
	for _, t := range tests {
		testNames = append(testNames, t.Resolved.GetEntity().GetName())
	}

	buildArtifactsURL := d.cfg.BuildArtifactsURL()
	if buildArtifactsURL == "" {
		buildArtifactsURL = dutInfo.GetDefaultBuildArtifactsUrl()
	}

	devservers := append([]string(nil), d.cfg.Devservers()...)
	if url, ok := d.cc.Conn().Services().EphemeralDevserverURL(); ok {
		devservers = append(devservers, url)
	}

	var tlwServer, tlwSelfName string
	if addr, ok := d.cc.Conn().Services().TLWAddr(); ok {
		tlwServer = addr.String()
		tlwSelfName = d.cfg.Target()
	}

	var dutServer string
	if addr, ok := d.cc.Conn().Services().DUTServerAddr(); ok {
		dutServer = addr.String()
	}

	bcfg := &protocol.BundleConfig{}
	rcfg := &protocol.RunConfig{
		Tests: testNames,
		Dirs: &protocol.RunDirectories{
			DataDir: d.cfg.LocalDataDir(),
			OutDir:  d.cfg.LocalOutDir(),
			TempDir: d.cfg.LocalTempDir(),
		},
		Features: d.cfg.Features(dutInfo.GetFeatures()),
		ServiceConfig: &protocol.ServiceConfig{
			Devservers:  devservers,
			DutServer:   dutServer, // Only pass in DUT server for local tests.
			TlwServer:   tlwServer,
			TlwSelfName: tlwSelfName,
		},
		DataFileConfig: &protocol.DataFileConfig{
			DownloadMode:      d.cfg.DownloadMode().Proto(),
			BuildArtifactsUrl: buildArtifactsURL,
		},
		StartFixtureState: state,
		HeartbeatInterval: ptypes.DurationProto(heartbeatInterval),
		WaitUntilReady:    d.cfg.WaitUntilReady(),
	}
	return bcfg, rcfg
}

func (d *Driver) newConfigsForRemoteTests(tests []*BundleEntity, dutInfo *protocol.DUTInfo, remoteDevservers []string) (*protocol.BundleConfig, *protocol.RunConfig, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}

	var testNames []string
	for _, t := range tests {
		testNames = append(testNames, t.Resolved.GetEntity().GetName())
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

	buildArtifactsURL := d.cfg.BuildArtifactsURL()
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
		HeartbeatInterval: ptypes.DurationProto(heartbeatInterval),
	}
	return bcfg, rcfg, nil
}

func (d *Driver) newRunFixtureConfig() (*bundle.RunFixtureConfig, error) {
	var tlwServer string
	if addr, ok := d.cc.Conn().Services().TLWAddr(); ok {
		tlwServer = addr.String()
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
		BuildArtifactsUrl: d.cfg.BuildArtifactsURL(),
		DownloadMode:      dm,
	}, nil
}

func splitTests(tests []*BundleEntity) (localTests, remoteTests []*BundleEntity, err error) {
	for _, t := range tests {
		switch t.Resolved.GetHops() {
		case 0:
			remoteTests = append(remoteTests, t)
		case 1:
			localTests = append(localTests, t)
		default:
			return nil, nil, errors.Errorf("unsupported hop %d for test %s", t.Resolved.GetHops(), t.Resolved.GetEntity().GetName())
		}
	}
	return localTests, remoteTests, nil
}

// nopDiagnose is a DiagnoseFunc that does nothing.
func nopDiagnose(ctx context.Context, outDir string) string {
	return ""
}

// runTestsOnceFunc is a function to run tests once.
type runTestsOnceFunc func(ctx context.Context, tests []*BundleEntity) ([]*resultsjson.Result, error)

// runTestsWithRetry runs tests in a loop. If runTestsOnce returns insufficient
// results, it calls beforeRetry, followed by runTestsOnce again to restart
// testing.
// Additionally, this will honor the retry CLI flag.
func runTestsWithRetry(ctx context.Context, allTests []*BundleEntity, runTestsOnce runTestsOnceFunc, retryn int) ([]*resultsjson.Result, error) {
	var allResults []*resultsjson.Result
	unstarted := make(map[string]struct{})

	logging.Infof(ctx, "Allowing up to %v retries", retryn)
	retries := make(map[string]int)
	for _, t := range allTests {
		unstarted[t.Resolved.GetEntity().GetName()] = struct{}{}
		retries[t.Resolved.GetEntity().GetName()] = retryn
	}

	for {
		// Compute tests to run.
		tests := make([]*BundleEntity, 0, len(unstarted))
		for _, t := range allTests {
			if _, ok := unstarted[t.Resolved.GetEntity().GetName()]; ok {
				tests = append(tests, t)
			}
		}

		// Run tests once.
		results, err := runTestsOnce(ctx, tests)
		if err != nil {
			allResults = append(allResults, results...)
			return allResults, err
		}

		// Abort to avoid infinite retries if no test ran in the last attempt.
		// Note: this needs to happen above the results modifications as if there
		// is 1 failing test & we strip that fail then we return rather than retry.
		if len(results) == 0 {
			return allResults, errors.New("no test ran in the last attempt")
		}

		// Update the results and unstarted list based on retries/failures.
		for _, r := range results {
			if len(r.Errors) > 0 && retries[r.Test.Name] > 0 {
				logging.Infof(ctx, "Retrying %v due to failure", r.Test.Name)
				retries[r.Test.Name]--
				continue
			}
			allResults = append(allResults, r)
			delete(unstarted, r.Test.Name)
		}

		// Return success if we ran all tests and there are no more retries.
		if len(unstarted) == 0 {
			return allResults, nil
		}

		logging.Infof(ctx, "Trying to run %v remaining test(s)", len(unstarted))
	}
}
