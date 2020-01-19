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
// If cfg.checkTestDeps is checkTestDepsAuto, it may be updated (e.g. if it's not
// possible to check dependencies).
func getSoftwareFeatures(ctx context.Context, cfg *Config) error {
	// Don't collect features if we're not checking deps or if we already have feature lists.
	if !cfg.checkTestDeps || len(cfg.availableSoftwareFeatures) > 0 ||
		len(cfg.unavailableSoftwareFeatures) > 0 {
		return nil
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
		if cfg.checkTestDeps {
			return errors.New("can't check test deps; no software features reported by DUT")
		}
		cfg.Logger.Debug("Test software dependencies will not be checked")
		cfg.checkTestDeps = false
		return nil
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
