// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"sort"

	"chromiumos/tast/internal/bundle/bundleclient"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
)

// testEntitiesToRun returns a sorted list of tests to run for the given names.
func testEntitiesToRun(allEntities []*protocol.ResolvedEntity, names []string) []*protocol.ResolvedEntity {
	nameSet := make(map[string]struct{})
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	var tests []*protocol.ResolvedEntity
	for _, t := range allEntities {
		if t.Entity.Type != protocol.EntityType_TEST {
			continue
		}
		if _, ok := nameSet[t.GetEntity().GetName()]; ok {
			tests = append(tests, t)
		}
	}
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].Hops != tests[j].Hops {
			return tests[i].Hops > tests[j].Hops
		}
		return tests[i].Entity.Name < tests[j].Entity.Name
	})
	return tests
}

// runTestsRecursive runs tests per rcfg and scfg and writes responses to srv.
// If target bundle is specified in bundleParams, it runs tests on the target
// bundle too.
//
// If an error is encountered in the test harness (as opposed to in a test), an
// error is returned. Otherwise, nil is returned (test errors will be reported
// via EntityError control messages).
func runTestsRecursive(ctx context.Context, srv protocol.TestService_RunTestsServer, rcfg *protocol.RunConfig, scfg *StaticConfig, bundleParams *protocol.BundleInitParams) (retErr error) {
	ctx = testcontext.WithPrivateData(ctx, testcontext.PrivateData{
		WaitUntilReady: rcfg.GetWaitUntilReady(),
	})
	bcfg := bundleParams.GetBundleConfig()

	ew := newEventWriter(srv)

	hbw := newHeartbeatWriter(ew)
	defer hbw.Stop()

	ctx = logging.AttachLoggerNoPropagation(ctx, logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(func(msg string) {
		ew.RunLog(msg)
	})))

	var cl *bundleclient.Client
	if target := bcfg.GetPrimaryTarget(); target != nil {
		var err error
		cl, err = bundleclient.New(ctx, bcfg.GetPrimaryTarget(), scfg.registry.Name(), &protocol.HandshakeRequest{
			BundleInitParams: &protocol.BundleInitParams{
				Vars: bundleParams.Vars,
			},
		})
		if err != nil {
			return err
		}
		defer func() {
			if err := cl.Close(ctx); err != nil && retErr == nil {
				retErr = err
			}
		}()
	}
	es, err := listEntitiesRecursive(ctx, scfg.registry, rcfg.GetFeatures(), cl)
	if err != nil {
		return err
	}
	testEntities := testEntitiesToRun(es, rcfg.GetTests())

	connEnv, err := setUpConnection(ctx, scfg, rcfg, bcfg)
	if err != nil {
		return err
	}
	defer connEnv.close(ctx)

	// Set up environment and create pcfg early so that we can run remote
	// fixtures for local tests.
	env, err := setUpTestEnvironment(ctx, scfg, rcfg, bcfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := env.close(ctx); err != nil && retErr != nil {
			retErr = err
		}
	}()

	internalTests := make(map[string]*testing.TestInstance)
	for _, t := range scfg.registry.AllTests() {
		internalTests[t.Name] = t
		if t.Timeout == 0 {
			t.Timeout = scfg.defaultTestTimeout
		}
	}
	pcfg := &planner.Config{
		Dirs:             rcfg.GetDirs(),
		Features:         rcfg.GetFeatures(),
		Service:          rcfg.GetServiceConfig(),
		DataFile:         rcfg.GetDataFileConfig(),
		RemoteData:       connEnv.rd,
		TestHook:         scfg.testHook,
		BeforeDownload:   scfg.beforeDownload,
		Tests:            internalTests,
		Fixtures:         scfg.registry.AllFixtures(),
		StartFixtureName: rcfg.GetStartFixtureState().GetName(),
		StartFixtureImpl: &stubFixture{setUpErrors: rcfg.GetStartFixtureState().GetErrors()},
		ExternalTarget: &planner.ExternalTarget{
			Device: bundleParams.GetBundleConfig().GetPrimaryTarget(),
			Config: rcfg.GetTarget(),
			Bundle: scfg.registry.Name(),
		},
	}

	var internal []*protocol.ResolvedEntity
	var external []*protocol.ResolvedEntity
	for _, t := range testEntities {
		if t.Hops == 0 {
			internal = append(internal, t)
		} else {
			external = append(external, t)
		}
	}
	// Run all the externalTests with Hops > 0 (i.e. local tests).
	if err := planner.RunTests(ctx, external, ew, pcfg); err != nil {
		return command.NewStatusErrorf(statusError, "run failed: %v", err)
	}
	// Run all the tests with Hops = 0.
	if err := planner.RunTests(ctx, internal, ew, pcfg); err != nil {
		return command.NewStatusErrorf(statusError, "run failed: %v", err)
	}
	return nil
}
