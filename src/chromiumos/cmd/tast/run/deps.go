// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"errors"
	"strings"

	"chromiumos/tast/bundle"
	"chromiumos/tast/runner"
	"chromiumos/tast/timing"
)

// getSoftwareFeatures executes local_test_runner on the DUT to get a list of
// available software features. These features are used to check tests' dependencies.
// If cfg.checkTestDeps is checkTestDepsAuto, it may be updated (e.g. if it's not
// possible to check dependencies).
func getSoftwareFeatures(ctx context.Context, cfg *Config) error {
	// Don't collect features if we're not checking deps or if we already have feature lists.
	if cfg.checkTestDeps == checkTestDepsNever || len(cfg.availableSoftwareFeatures) > 0 ||
		len(cfg.unavailableSoftwareFeatures) > 0 {
		return nil
	}

	// If the user-supplied test patterns are wildcards (or more likely, literal test names),
	// assume that they really want to run those particular tests and skip checking dependencies.
	if cfg.checkTestDeps == checkTestDepsAuto &&
		bundle.GetTestPatternType(cfg.Patterns) == bundle.TestPatternWildcard {
		cfg.Logger.Debug("Test software dependencies not checked for wildcard/literal test patterns")
		cfg.checkTestDeps = checkTestDepsNever
		return nil
	}

	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("get_software_features")
		defer st.End()
	}
	cfg.Logger.Debug("Getting software features supported by DUT")

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return err
	}
	handle, err := startLocalRunner(ctx, cfg, hst, nil, &runner.Args{
		Mode: runner.GetSoftwareFeaturesMode,
		GetSoftwareFeaturesArgs: runner.GetSoftwareFeaturesArgs{
			ExtraUSEFlags: cfg.extraUSEFlags,
		},
	})
	if err != nil {
		return err
	}
	defer handle.Close(ctx)
	var res runner.GetSoftwareFeaturesResult
	if err = readLocalRunnerOutput(ctx, handle, &res); err != nil {
		return err
	}

	// If both the available and unavailable lists were empty, then the DUT doesn't
	// know about its features (e.g. because it's a non-test image and doesn't have
	// a listing of relevant USE flags).
	if len(res.Available) == 0 && len(res.Unavailable) == 0 {
		cfg.Logger.Debug("No software features reported by DUT -- non-test image?")
		if cfg.checkTestDeps == checkTestDepsAlways {
			return errors.New("can't check test deps; no software features reported by DUT")
		}
		cfg.Logger.Debug("Test software dependencies will not be checked")
		cfg.checkTestDeps = checkTestDepsNever
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
	switch cfg.checkTestDeps {
	case checkTestDepsAlways, checkTestDepsAuto:
		args.RunTestsArgs.CheckSoftwareDeps = true
	case checkTestDepsNever:
		args.RunTestsArgs.CheckSoftwareDeps = false
	}
	args.RunTestsArgs.AvailableSoftwareFeatures = cfg.availableSoftwareFeatures
	args.RunTestsArgs.UnavailableSoftwareFeatures = cfg.unavailableSoftwareFeatures
}
