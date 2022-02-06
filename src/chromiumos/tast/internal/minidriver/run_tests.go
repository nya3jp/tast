// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package minidriver provides function to run tests in an external bundle.
package minidriver

import (
	"context"
	"errors"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/types/known/durationpb"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/bundleclient"
	"chromiumos/tast/internal/minidriver/diagnose"
	"chromiumos/tast/internal/minidriver/failfast"
	"chromiumos/tast/internal/minidriver/processor"
	"chromiumos/tast/internal/minidriver/target"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/reporting"
	"chromiumos/tast/internal/run/resultsjson"
)

const (
	// HeartbeatInterval is interval for heartbeat messages.
	HeartbeatInterval = time.Second
)

// Driver is the main struct for running tests.
type Driver struct {
	cfg *Config
	cc  *target.ConnCache
}

// NewDriver creates a new Driver.
func NewDriver(cfg *Config, cc *target.ConnCache) *Driver {
	return &Driver{
		cfg: cfg,
		cc:  cc,
	}
}

// Config provides configurations for running tests.
type Config struct {
	Retries               int
	ResDir                string
	Devservers            []string
	Target                string
	LocalDataDir          string
	LocalOutDir           string
	LocalTempDir          string
	LocalBundleDir        string
	DownloadMode          protocol.DownloadMode
	WaitUntilReady        bool
	SystemServicesTimeout time.Duration
	CheckTestDeps         bool
	TestVars              map[string]string
	MaybeMissingVars      string

	DebuggerPort int
	Proxy        bool

	DUTFeatures *protocol.DUTFeatures
	Factory     HandlersFactory

	BuildArtifactsURL string

	// Recursive specifies whether to run tests recursively.
	// This must be true to support remote fixture.
	Recursive bool
}

// RunLocalTests runs external tests with retry.
func (d *Driver) RunLocalTests(ctx context.Context, bundle string, tests []string, state *protocol.StartFixtureState) ([]*resultsjson.Result, error) {
	runTestsOnce := func(ctx context.Context, tests []string) ([]*resultsjson.Result, error) {
		return d.runLocalTestsOnce(ctx, bundle, tests, state)
	}
	return RunTestsWithRetry(ctx, tests, runTestsOnce, d.cfg.Retries)
}

// HandlersFactory is a type which creates processor handlers.
type HandlersFactory func(ctx context.Context, cc *target.ConnCache) (context.Context, []processor.Handler)

// NewRootHandlersFactory creates a new factory for CLI.
func NewRootHandlersFactory(resDir string, counter *failfast.Counter, client *reporting.RPCClient) HandlersFactory {
	return func(ctx context.Context, cc *target.ConnCache) (context.Context, []processor.Handler) {
		multiplexer := logging.NewMultiLogger()
		ctx = logging.AttachLogger(ctx, multiplexer)

		pull := func(src, dst string) error {
			return linuxssh.GetAndDeleteFile(ctx, cc.Conn().SSHConn(), src, dst, linuxssh.PreserveSymlinks)
		}
		return ctx, []processor.Handler{
			processor.NewLoggingHandler(resDir, multiplexer, client),
			processor.NewTimingHandler(),
			processor.NewStreamedResultsHandler(resDir),
			processor.NewRPCResultsHandler(client),
			processor.NewFailFastHandler(counter),
			// copyOutputHandler should come last as it can block RunEnd for a while.
			processor.NewCopyOutputHandler(pull),
		}
	}
}

// NewIntermediateHandlersFactory creates a new factory.
// When tests are run recursively, intermediate component (i.e. remote bundle)
// will use this factory.
func NewIntermediateHandlersFactory(resDir string, counter *failfast.Counter, pass func(*protocol.RunTestsResponse) error, handle processor.HandleStack) HandlersFactory {
	return func(ctx context.Context, cc *target.ConnCache) (context.Context, []processor.Handler) {
		multiplexer := logging.NewMultiLogger()
		ctx = logging.AttachLogger(ctx, multiplexer)
		pull := func(src, dst string) error {
			return linuxssh.GetAndDeleteFile(ctx, cc.Conn().SSHConn(), src, dst, linuxssh.PreserveSymlinks)
		}
		return ctx, []processor.Handler{
			processor.NewFailFastHandler(counter),
			processor.NewStackOperationHandler(handle),
			// copyOutputHandler should come last as it can block RunEnd for a while.
			processor.NewCopyAndPassThroughHandler(pass, pull),
		}
	}
}

func (d *Driver) runLocalTestsOnce(ctx context.Context, bundle string, tests []string, state *protocol.StartFixtureState) ([]*resultsjson.Result, error) {
	if err := d.cc.EnsureConn(ctx); err != nil {
		return nil, err
	}

	bcfg, rcfg := d.newConfigsForLocalTests(tests, state)

	diag := func(ctx context.Context, outDir string) string {
		if ctxutil.DeadlineBefore(ctx, time.Now().Add(target.SSHPingTimeout)) {
			return ""
		}
		if err := d.cc.Conn().SSHConn().Ping(ctx, target.SSHPingTimeout); err == nil {
			return ""
		}
		return "Lost SSH connection: " + diagnose.SSHDrop(ctx, d.cc, outDir)
	}

	ctx, hs := d.cfg.Factory(ctx, d.cc)

	proc := processor.New(d.cfg.ResDir, diag, hs)
	cl := bundleclient.NewLocal(bundle, d.cfg.LocalBundleDir, d.cfg.Proxy, d.cc)
	cl.RunTests(ctx, bcfg, rcfg, proc, d.cfg.Recursive)
	return proc.Results(), proc.FatalError()
}

func (d *Driver) newConfigsForLocalTests(tests []string, state *protocol.StartFixtureState) (*protocol.BundleConfig, *protocol.RunConfig) {
	devservers := append([]string(nil), d.cfg.Devservers...)
	if url, ok := d.cc.Conn().Services().EphemeralDevserverURL(); ok {
		devservers = append(devservers, url)
	}

	var tlwServer, tlwSelfName string
	if addr, ok := d.cc.Conn().Services().TLWAddr(); ok {
		tlwServer = addr.String()
		tlwSelfName = d.cfg.Target
	}

	var dutServer string
	if addr, ok := d.cc.Conn().Services().DUTServerAddr(); ok {
		dutServer = addr.String()
	}

	bcfg := &protocol.BundleConfig{}
	rcfg := &protocol.RunConfig{
		Tests: tests,
		Dirs: &protocol.RunDirectories{
			DataDir: d.cfg.LocalDataDir,
			OutDir:  d.cfg.LocalOutDir,
			TempDir: d.cfg.LocalTempDir,
		},
		Features: &protocol.Features{
			CheckDeps: d.cfg.CheckTestDeps,
			Infra: &protocol.InfraFeatures{
				Vars:             d.cfg.TestVars,
				MaybeMissingVars: d.cfg.MaybeMissingVars,
			},
			Dut: d.cfg.DUTFeatures,
		},
		ServiceConfig: &protocol.ServiceConfig{
			Devservers:  devservers,
			DutServer:   dutServer, // Only pass in DUT server for local tests.
			TlwServer:   tlwServer,
			TlwSelfName: tlwSelfName,
		},
		DataFileConfig: &protocol.DataFileConfig{
			DownloadMode:      d.cfg.DownloadMode,
			BuildArtifactsUrl: d.cfg.BuildArtifactsURL,
		},
		StartFixtureState:     state,
		HeartbeatInterval:     ptypes.DurationProto(HeartbeatInterval),
		WaitUntilReady:        d.cfg.WaitUntilReady,
		SystemServicesTimeout: durationpb.New(d.cfg.SystemServicesTimeout),
		DebugPort:             uint32(d.cfg.DebuggerPort),
	}
	return bcfg, rcfg
}

// runTestsOnceFunc is a function to run tests once.
type runTestsOnceFunc func(ctx context.Context, tests []string) ([]*resultsjson.Result, error)

// RunTestsWithRetry runs tests in a loop. If runTestsOnce returns insufficient
// results, it calls beforeRetry, followed by runTestsOnce again to restart
// testing.
// Additionally, this will honor the retry CLI flag.
func RunTestsWithRetry(ctx context.Context, allTests []string, runTestsOnce runTestsOnceFunc, retryn int) ([]*resultsjson.Result, error) {
	var allResults []*resultsjson.Result
	unstarted := make(map[string]struct{})

	logging.Infof(ctx, "Allowing up to %v retries", retryn)
	retries := make(map[string]int)
	for _, t := range allTests {
		unstarted[t] = struct{}{}
		retries[t] = retryn
	}

	for {
		// Compute tests to run.
		tests := make([]string, 0, len(unstarted))
		for _, t := range allTests {
			if _, ok := unstarted[t]; ok {
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
			return allResults, ErrNoTestRanInLastAttempt
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

// ErrNoTestRanInLastAttempt indicates no test was ran in the last attempt.
var ErrNoTestRanInLastAttempt = errors.New("no test ran in the last attempt")
