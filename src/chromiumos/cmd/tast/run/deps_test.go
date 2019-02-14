// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"chromiumos/tast/bundle"
	"chromiumos/tast/runner"
)

// writeGetSoftwareFeaturesResult writes runner.GetSoftwareFeaturesResult to w.
func writeGetSoftwareFeaturesResult(w io.Writer, avail, unavail []string) error {
	res := runner.GetSoftwareFeaturesResult{Available: avail, Unavailable: unavail}
	return json.NewEncoder(w).Encode(&res)
}

// checkRunnerTestDepsArgs calls setRunnerTestDepsArgs using cfg and verifies
// that it sets runner args as specified per checkDeps, avail, and unavail.
func checkRunnerTestDepsArgs(t *testing.T, cfg *Config, checkDeps bool, avail, unavail []string) {
	args := runner.Args{
		Mode:     runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{},
	}
	setRunnerTestDepsArgs(cfg, &args)

	exp := runner.RunTestsArgs{
		BundleArgs: bundle.RunTestsArgs{
			CheckSoftwareDeps:           checkDeps,
			AvailableSoftwareFeatures:   avail,
			UnavailableSoftwareFeatures: unavail,
		},
	}
	if !reflect.DeepEqual(*args.RunTests, exp) {
		t.Errorf("setRunnerTestDepsArgs(%+v) set %+v; want %+v", cfg, *args.RunTests, exp)
	}
}

func TestGetSoftwareFeaturesAlways(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// With "always", features returned by the runner should be passed through
	// and dependencies should be checked.
	avail := []string{"dep1", "dep2"}
	unavail := []string{"dep3"}
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.GetSoftwareFeaturesMode,
			GetSoftwareFeatures: &runner.GetSoftwareFeaturesArgs{
				ExtraUSEFlags: td.cfg.extraUSEFlags,
			},
			ExtraUSEFlagsDeprecated: td.cfg.extraUSEFlags,
		})

		writeGetSoftwareFeaturesResult(stdout, avail, unavail)
		return 0
	}
	td.cfg.checkTestDeps = checkTestDepsAlways
	td.cfg.extraUSEFlags = []string{"use1", "use2"}
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, true, avail, unavail)

	// Change the features reported by local_test_runner and call getSoftwareFeature again.
	// Since we already have the features, we shouldn't run local_test_runner again and should
	// continue using the original features.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		writeGetSoftwareFeaturesResult(stdout, []string{"new1"}, []string{"new2"})
		return 0
	}
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed on second call: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, true, avail, unavail)
}

func TestGetSoftwareFeaturesNever(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
	td.cfg.checkTestDeps = checkTestDepsNever
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil)
}

func TestGetSoftwareFeaturesAutoAttrExpr(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// When "auto" is used in conjunction with an attribute-expression-based test
	// pattern, dependencies should be checked.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:                runner.GetSoftwareFeaturesMode,
			GetSoftwareFeatures: &runner.GetSoftwareFeaturesArgs{},
		})
		writeGetSoftwareFeaturesResult(stdout, []string{"foo"}, []string{})
		return 0
	}
	td.cfg.Patterns = []string{"(bvt)"} // attr expr needed to check deps with "auto"
	td.cfg.checkTestDeps = checkTestDepsAuto
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, true, []string{"foo"}, nil)
}

func TestGetSoftwareFeaturesAutoSpecificTest(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// With "auto" and a pattern that specifies a particular test
	// (rather than a attribute expression), the runner shouldn't be called and
	// dependencies shouldn't be checked.
	td.cfg.checkTestDeps = checkTestDepsAuto
	td.cfg.Patterns = []string{"pkg.Test"}
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil)
}

func TestGetSoftwareFeaturesAutoNoFeatures(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// "auto" should be downgraded to "never" if the runner didn't report knowing
	// about any features at all (probably because it's running on a non-test system image).
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:                runner.GetSoftwareFeaturesMode,
			GetSoftwareFeatures: &runner.GetSoftwareFeaturesArgs{},
		})
		writeGetSoftwareFeaturesResult(stdout, []string{}, []string{})
		return 0
	}
	td.cfg.Patterns = []string{"(bvt)"} // attr expr needed to check deps with "auto"
	td.cfg.checkTestDeps = checkTestDepsAuto
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil)
}

func TestGetSoftwareFeaturesAlwaysNoFeatures(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// "always" should fail if the runner doesn't know about any features.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:                runner.GetSoftwareFeaturesMode,
			GetSoftwareFeatures: &runner.GetSoftwareFeaturesArgs{},
		})
		writeGetSoftwareFeaturesResult(stdout, []string{}, []string{})
		return 0
	}
	td.cfg.checkTestDeps = checkTestDepsAlways
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err == nil {
		t.Fatalf("getSoftwareFeatures(%+v) succeeded unexpectedly", td.cfg)
	}
}
