// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"

	"chromiumos/tast/internal/minidriver"
	"chromiumos/tast/internal/minidriver/failfast"
	"chromiumos/tast/internal/minidriver/target"
	"chromiumos/tast/internal/planner/internal/fixture"
	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// ExternalTarget specifies the external target bundle to run.
type ExternalTarget struct {
	Device *protocol.TargetDevice
	Config *protocol.RunTargetConfig
	Bundle string
}

// runExternalTests runs tests in primary target.
// It sends a request to run the tests to the target bundle, and handles fixture
// stack operation requests.
// It returns an error If a test could not be run, or an internal framework
// error has happened.
// It returns unstarted tests.
// External tests might not fully finish in case Stack Reset failure.
// unstarted tests are returned for allowing retry unstarted tests.
func runExternalTests(ctx context.Context, names []string, stack *fixture.CombinedStack, pcfg *Config, out output.Stream) (unstarted []string, err error) {
	scfg := &target.ServiceConfig{
		Devservers: pcfg.ExternalTarget.Config.GetDevservers(),
		TLWServer:  pcfg.Service.GetTlwServer(),

		UseEphemeralDevserver: pcfg.Service.GetUseEphemeralDevservers(),
		TastDir:               pcfg.Service.GetTastDir(),
		ExtraAllowedBuckets:   pcfg.Service.GetExtraAllowedBuckets(),
		DebuggerPorts:         []int{int(pcfg.ExternalTarget.Config.GetDebugPort())},
	}
	tcfg := &target.Config{
		SSHConfig:     pcfg.ExternalTarget.Device.GetDutConfig().GetSshConfig(),
		TastVars:      pcfg.Features.GetInfra().GetVars(),
		ServiceConfig: scfg,
	}
	cc, err := target.NewConnCache(ctx, tcfg, pcfg.ExternalTarget.Device.GetDutConfig().GetSshConfig().GetConnectionSpec(), "")
	if err != nil {
		return nil, err
	}

	fixtureServer := fixture.NewStackServer(&fixture.StackServerConfig{
		Out:    out,
		Stack:  stack,
		OutDir: pcfg.Dirs.GetOutDir(),
		CloudStorage: testing.NewCloudStorage(
			pcfg.Service.GetDevservers(),
			pcfg.Service.GetTlwServer(),
			pcfg.Service.GetTlwSelfName(),
			pcfg.Service.GetDutServer(),
			pcfg.DataFile.GetBuildArtifactsUrl(),
		),
		RemoteData: pcfg.RemoteData,
	})

	counter := failfast.NewCounter(int(pcfg.ExternalTarget.Config.GetMaxTestFailures()))
	factory := minidriver.NewIntermediateHandlersFactory(pcfg.Dirs.GetOutDir(), counter, out.ExternalEvent, fixtureServer.Handle)

	cfg := &minidriver.Config{
		Retries:        int(pcfg.ExternalTarget.Config.GetRetries()),
		ResDir:         pcfg.Dirs.GetOutDir(),
		Devservers:     pcfg.Service.GetDevservers(),
		Target:         pcfg.ExternalTarget.Device.GetDutConfig().GetSshConfig().GetConnectionSpec(),
		LocalDataDir:   pcfg.ExternalTarget.Config.GetDirs().GetDataDir(),
		LocalOutDir:    pcfg.ExternalTarget.Config.GetDirs().GetOutDir(),
		LocalTempDir:   pcfg.ExternalTarget.Config.GetDirs().GetTempDir(),
		LocalBundleDir: pcfg.ExternalTarget.Device.GetBundleDir(),
		DownloadMode:   pcfg.DataFile.GetDownloadMode(),

		WaitUntilReady:   pcfg.ExternalTarget.Config.GetWaitUntilReady(),
		CheckTestDeps:    pcfg.Features.GetCheckDeps(),
		TestVars:         pcfg.Features.GetInfra().GetVars(),
		MaybeMissingVars: pcfg.Features.GetInfra().GetMaybeMissingVars(),

		DebuggerPort: int(pcfg.ExternalTarget.Config.GetDebugPort()),
		Proxy:        pcfg.ExternalTarget.Config.GetProxy(),

		DUTFeatures: pcfg.Features.GetDut(),
		Factory:     factory,

		Recursive: true,
	}

	d := minidriver.NewDriver(cfg, cc)

	startFixture := stack.Top()

	jsonResults, err := d.RunLocalTests(ctx, pcfg.ExternalTarget.Bundle, names, startFixture)
	if err == minidriver.ErrNoTestRanInLastAttempt {
		if len(jsonResults) == 0 {
			return nil, err
		}
		// Fixture failure stopped local tests running.
	} else if err != nil {
		return nil, err
	}
	startedSet := make(map[string]struct{})
	for _, t := range jsonResults {
		startedSet[t.Name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := startedSet[name]; !ok {
			unstarted = append(unstarted, name)
		}
	}
	return unstarted, nil
}
