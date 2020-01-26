// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"errors"
	"strings"

	"chromiumos/tast/internal/runner"
	"chromiumos/tast/timing"
)

// getSoftwareFeatures executes local_test_runner on the DUT to get a list of
// available software features. These features are used to check tests' dependencies.
// This updates cfg.availableSoftwareFeatures and cfg.unavailableSoftwareFeatures.
// Thus, calling this twice won't work.
func getSoftwareFeatures(ctx context.Context, cfg *Config) error {
	if !cfg.checkTestDeps {
		return nil
	}
	if len(cfg.availableSoftwareFeatures) > 0 || len(cfg.unavailableSoftwareFeatures) > 0 {
		return errors.New("getSoftwareFeatures is already called")
	}

	ctx, st := timing.Start(ctx, "get_software_features")
	defer st.End()
	cfg.Logger.Debug("Getting software features supported by DUT")

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return err
	}

	var res runner.GetSoftwareFeaturesResult
	if err := runTestRunnerCommand(
		localRunnerCommand(ctx, cfg, hst),
		&runner.Args{
			Mode: runner.GetSoftwareFeaturesMode,
			GetSoftwareFeatures: &runner.GetSoftwareFeaturesArgs{
				ExtraUSEFlags: cfg.extraUSEFlags,
			},
		},
		&res,
	); err != nil {
		return err
	}

	// If both the available and unavailable lists were empty, then the DUT doesn't
	// know about its features (e.g. because it's a non-test image and doesn't have
	// a listing of relevant USE flags).
	if len(res.Available) == 0 && len(res.Unavailable) == 0 {
		cfg.Logger.Debug("No software features reported by DUT -- non-test image?")
		return errors.New("can't check test deps; no software features reported by DUT")
	}

	for _, warn := range res.Warnings {
		cfg.Logger.Log(warn)
	}
	cfg.Logger.Debug("Software features supported by DUT: ", strings.Join(res.Available, " "))
	cfg.availableSoftwareFeatures = res.Available
	cfg.unavailableSoftwareFeatures = res.Unavailable
	return nil
}

func setRunnerTestDepsArgs(cfg *Config, args *runner.Args) {
	args.RunTests.BundleArgs.CheckSoftwareDeps = cfg.checkTestDeps
	args.RunTests.BundleArgs.AvailableSoftwareFeatures = cfg.availableSoftwareFeatures
	args.RunTests.BundleArgs.UnavailableSoftwareFeatures = cfg.unavailableSoftwareFeatures
}
