// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"fmt"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/ssh"
)

// ListTests returns a list of all tests (including both local and remote tests).
func ListTests(ctx context.Context, cfg *config.Config, dutInfo *protocol.DUTInfo, cc *target.ConnCache) ([]*protocol.ResolvedEntity, error) {
	conn, err := cc.Conn(ctx)
	if err != nil {
		return nil, err
	}
	localTests, err := ListLocalTests(ctx, cfg, dutInfo, conn.SSHConn())
	if err != nil {
		return nil, err
	}
	remoteTests, err := listRemoteTests(ctx, cfg, dutInfo)
	if err != nil {
		return nil, err
	}
	return append(localTests, remoteTests...), nil
}

// ListLocalTests returns a list of local tests to run.
func ListLocalTests(ctx context.Context, cfg *config.Config, dutInfo *protocol.DUTInfo, hst *ssh.Conn) ([]*protocol.ResolvedEntity, error) {
	entities, err := runListTestsCommand(ctx, localRunnerCommand(cfg, hst), cfg, dutInfo, cfg.LocalBundleGlob())
	if err != nil {
		return nil, err
	}
	// It is our responsibility to adjust hops.
	for _, e := range entities {
		e.Hops++
	}
	return entities, nil
}

// ListLocalFixtures returns a map from bundle to fixtures.
func ListLocalFixtures(ctx context.Context, cfg *config.Config, hst *ssh.Conn) (map[string][]*protocol.Entity, error) {
	var res jsonprotocol.RunnerListFixturesResult
	if err := runTestRunnerCommand(
		ctx,
		localRunnerCommand(cfg, hst), &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerListFixturesMode,
			ListFixtures: &jsonprotocol.RunnerListFixturesArgs{
				BundleGlob: cfg.LocalBundleGlob(),
			},
		}, &res); err != nil {
		return nil, fmt.Errorf("listing local fixtures: %v", err)
	}
	return convertFixtureMap(res.Fixtures)
}

// listRemoteTests returns a list of remote tests to run.
func listRemoteTests(ctx context.Context, cfg *config.Config, dutInfo *protocol.DUTInfo) ([]*protocol.ResolvedEntity, error) {
	return runListTestsCommand(
		ctx, remoteRunnerCommand(cfg), cfg, dutInfo, cfg.RemoteBundleGlob())
}

// listRemoteFixtures returns a map from bundle to fixtures.
func listRemoteFixtures(ctx context.Context, cfg *config.Config) (map[string][]*protocol.Entity, error) {
	var res jsonprotocol.RunnerListFixturesResult
	if err := runTestRunnerCommand(
		ctx,
		remoteRunnerCommand(cfg), &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerListFixturesMode,
			ListFixtures: &jsonprotocol.RunnerListFixturesArgs{
				BundleGlob: cfg.RemoteBundleGlob(),
			},
		}, &res); err != nil {
		return nil, fmt.Errorf("listing remote fixtures: %v", err)
	}
	return convertFixtureMap(res.Fixtures)
}

func convertFixtureMap(jsonFixtMap map[string][]*jsonprotocol.EntityInfo) (map[string][]*protocol.Entity, error) {
	protoFixtMap := make(map[string][]*protocol.Entity)
	for bundle, jsonFixts := range jsonFixtMap {
		protoFixts := make([]*protocol.Entity, len(jsonFixts))
		for i, jf := range jsonFixts {
			pf, err := jf.Proto()
			if err != nil {
				return nil, err
			}
			protoFixts[i] = pf
		}
		protoFixtMap[bundle] = protoFixts
	}
	return protoFixtMap, nil
}

func runListTestsCommand(ctx context.Context, cmd genericexec.Cmd, cfg *config.Config, dutInfo *protocol.DUTInfo, glob string) ([]*protocol.ResolvedEntity, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerListTestsMode,
		ListTests: &jsonprotocol.RunnerListTestsArgs{
			BundleArgs: jsonprotocol.BundleListTestsArgs{
				FeatureArgs: *jsonprotocol.FeatureArgsFromProto(cfg.Features(dutInfo.GetFeatures())),
				Patterns:    cfg.Patterns,
			},
			BundleGlob: glob,
		},
	}
	var res jsonprotocol.RunnerListTestsResult
	if err := runTestRunnerCommand(ctx, cmd, args, &res); err != nil {
		return nil, fmt.Errorf("listing tests %v: %v", cfg.Patterns, err)
	}
	tests := make([]*protocol.ResolvedEntity, len(res))
	for i, r := range res {
		t, err := r.Proto()
		if err != nil {
			return nil, err
		}
		tests[i] = t
	}
	return tests, nil
}
