// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runnerclient provides test_runner clients.
package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/driver/internal/drivercore"
	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// JSONClient is a JSON-protocol client to test_runner.
type JSONClient struct {
	cmd        genericexec.Cmd
	params     *protocol.RunnerInitParams
	msgTimeout time.Duration
	hops       int
}

// NewJSONClient creates a new JSONClient.
func NewJSONClient(cmd genericexec.Cmd, params *protocol.RunnerInitParams, msgTimeout time.Duration, hops int) *JSONClient {
	return &JSONClient{
		cmd:        cmd,
		params:     params,
		msgTimeout: msgTimeout,
		hops:       hops,
	}
}

// GetDUTInfo retrieves various DUT information needed for test execution.
func (c *JSONClient) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerGetDUTInfoMode,
		GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
			ExtraUSEFlags:       req.GetExtraUseFlags(),
			RequestDeviceConfig: true,
		},
	}

	var res jsonprotocol.RunnerGetDUTInfoResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return nil, errors.Wrap(err, "getting DUT info")
	}

	// If the software feature is empty, then the DUT doesn't know about its features
	// (e.g. because it's a non-test image and doesn't have a listing of relevant USE flags).
	if res.SoftwareFeatures == nil {
		return nil, errors.New("can't check test deps; no software features reported by DUT")
	}

	for _, warn := range res.Warnings {
		logging.Info(ctx, warn)
	}
	return res.Proto(), nil
}

// GetSysInfoState collects the sysinfo state of the DUT.
func (c *JSONClient) GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerGetSysInfoStateMode,
	}
	var res jsonprotocol.RunnerGetSysInfoStateResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return nil, errors.Wrap(err, "getting sysinfo state")
	}
	for _, warn := range res.Warnings {
		logging.Info(ctx, warn)
	}
	return res.Proto(), nil
}

// CollectSysInfo collects the sysinfo, considering diff from the given initial
// sysinfo state.
func (c *JSONClient) CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode:           jsonprotocol.RunnerCollectSysInfoMode,
		CollectSysInfo: &jsonprotocol.RunnerCollectSysInfoArgs{InitialState: *jsonprotocol.SysInfoStateFromProto(req.GetInitialState())},
	}
	var res jsonprotocol.RunnerCollectSysInfoResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return nil, errors.Wrap(err, "collecting sysinfo")
	}
	for _, warn := range res.Warnings {
		logging.Info(ctx, warn)
	}
	return res.Proto(), nil
}

// DownloadPrivateBundles downloads and installs a private test bundle archive
// corresponding to the target version, if one has not been installed yet.
func (c *JSONClient) DownloadPrivateBundles(ctx context.Context, req *protocol.DownloadPrivateBundlesRequest) error {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerDownloadPrivateBundlesMode,
		DownloadPrivateBundles: &jsonprotocol.RunnerDownloadPrivateBundlesArgs{
			Devservers:        req.GetServiceConfig().GetDevservers(),
			TLWServer:         req.GetServiceConfig().GetTlwServer(),
			DUTName:           req.GetServiceConfig().GetTlwSelfName(),
			BuildArtifactsURL: req.GetBuildArtifactUrl(),
		},
	}
	var res jsonprotocol.RunnerDownloadPrivateBundlesResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return errors.Wrap(err, "downloading private bundles")
	}
	for _, warn := range res.Messages {
		logging.Info(ctx, warn)
	}
	return nil
}

// ListTests enumerates tests matching patterns.
func (c *JSONClient) ListTests(ctx context.Context, patterns []string, features *protocol.Features) ([]*drivercore.BundleEntity, error) {
	fixtures, err := c.ListFixtures(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list fixtures for tests")
	}

	graph := newFixtureGraphFromBundleEntities(fixtures)

	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerListTestsMode,
		ListTests: &jsonprotocol.RunnerListTestsArgs{
			BundleArgs: jsonprotocol.BundleListTestsArgs{
				FeatureArgs: *jsonprotocol.FeatureArgsFromProto(features),
				Patterns:    patterns,
			},
			BundleGlob: c.params.GetBundleGlob(),
		},
	}
	var res jsonprotocol.RunnerListTestsResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return nil, errors.Wrap(err, "failed to list tests")
	}

	tests := make([]*drivercore.BundleEntity, len(res))
	for i, r := range res {
		e, err := r.Proto(int32(c.hops), graph.FindStart(r.Bundle, r.Fixture))
		if err != nil {
			return nil, err
		}
		tests[i] = &drivercore.BundleEntity{
			Bundle:   r.Bundle,
			Resolved: e,
		}
	}
	return tests, nil
}

// ListFixtures enumerates all fixtures.
func (c *JSONClient) ListFixtures(ctx context.Context) ([]*drivercore.BundleEntity, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerListFixturesMode,
		ListFixtures: &jsonprotocol.RunnerListFixturesArgs{
			BundleGlob: c.params.GetBundleGlob(),
		},
	}
	var res jsonprotocol.RunnerListFixturesResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return nil, errors.Wrap(err, "failed to list fixtures")
	}

	graph := newFixtureGraphFromListFixturesResult(&res)

	var fixtures []*drivercore.BundleEntity
	for bundle, fs := range res.Fixtures {
		for _, f := range fs {
			e, err := f.Proto()
			if err != nil {
				return nil, err
			}
			fixtures = append(fixtures, &drivercore.BundleEntity{
				Bundle: f.Bundle,
				Resolved: &protocol.ResolvedEntity{
					Entity:           e,
					Hops:             int32(c.hops),
					StartFixtureName: graph.FindStart(bundle, f.Fixture),
				},
			})
		}
	}

	// In JSON protocol, the order of fixtures is unstable. Sort them here
	// for better reproducibility.
	sort.Slice(fixtures, func(i, j int) bool {
		ea, eb := fixtures[i].Resolved.GetEntity(), fixtures[j].Resolved.GetEntity()
		if a, b := ea.GetLegacyData().GetBundle(), eb.GetLegacyData().GetBundle(); a != b {
			return a < b
		}
		return ea.GetName() < eb.GetName()
	})

	return fixtures, nil
}

func (c *JSONClient) runBatch(ctx context.Context, args *jsonprotocol.RunnerArgs, out interface{}) error {
	args.FillDeprecated()

	stdin, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal runner args: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := c.cmd.Run(ctx, nil, bytes.NewBuffer(stdin), &stdout, &stderr); err != nil {
		// Append the first line of stderr, which often contains useful info
		// for debugging to users.
		if split := bytes.SplitN(stderr.Bytes(), []byte(","), 2); len(split) > 0 {
			err = errors.Errorf("%v: %s", err, string(split[0]))
		}
		return err
	}
	if err := json.NewDecoder(&stdout).Decode(out); err != nil {
		return errors.Wrap(err, "failed to unmarshal runner response")
	}
	return nil
}
