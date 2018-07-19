// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"chromiumos/tast/bundle"
	"chromiumos/tast/runner"
)

// addGetSoftwareFeaturesResult registers a command in td.srvData.Srv
// to call local_test_runner with runner.GetSoftwareFeaturesMode.
func addGetSoftwareFeaturesResult(t *testing.T, td *localTestData,
	avail, unavail []string) (stdin *bytes.Buffer) {
	ob := bytes.Buffer{}
	res := runner.GetSoftwareFeaturesResult{Available: avail, Unavailable: unavail}
	if err := json.NewEncoder(&ob).Encode(&res); err != nil {
		t.Fatal(err)
	}
	return addLocalRunnerFakeCmd(td.srvData.Srv, 0, ob.Bytes(), nil)
}

// checkRunnerTestDepsArgs calls setRunnerTestDepsArgs using cfg and verifies
// that it sets runner args as specified per checkDeps, avail, and unavail.
func checkRunnerTestDepsArgs(t *testing.T, cfg *Config, checkDeps bool, avail, unavail []string) {
	var args runner.Args
	setRunnerTestDepsArgs(cfg, &args)

	exp := runner.RunTestsArgs{
		bundle.RunTestsArgs{
			CheckSoftwareDeps:           checkDeps,
			AvailableSoftwareFeatures:   avail,
			UnavailableSoftwareFeatures: unavail,
		},
	}
	if !reflect.DeepEqual(args.RunTestsArgs, exp) {
		t.Errorf("setRunnerTestDepsArgs(%+v) set %+v; want %+v", cfg, args.RunTestsArgs, exp)
	}
}

func TestGetSoftwareFeaturesAlways(t *testing.T) {
	td := newLocalTestData()
	defer td.close()

	// With "always", features returned by the runner should be passed through
	// and dependencies should be checked.
	avail := []string{"dep1", "dep2"}
	unavail := []string{"dep3"}
	stdin := addGetSoftwareFeaturesResult(t, td, avail, unavail)
	td.cfg.checkTestDeps = checkTestDepsAlways
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkArgs(t, stdin, &runner.Args{Mode: runner.GetSoftwareFeaturesMode})
	checkRunnerTestDepsArgs(t, &td.cfg, true, avail, unavail)

	// Change the features reported by local_test_runner and call getSoftwareFeature again.
	// Since we already have the features, we shouldn't run local_test_runner again and should
	// continue using the original features.
	addGetSoftwareFeaturesResult(t, td, []string{"new1"}, []string{"new2"})
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed on second call: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, true, avail, unavail)
}

func TestGetSoftwareFeaturesNever(t *testing.T) {
	td := newLocalTestData()
	defer td.close()

	// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
	td.cfg.checkTestDeps = checkTestDepsNever
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil)
}

func TestGetSoftwareFeaturesAutoAttrExpr(t *testing.T) {
	td := newLocalTestData()
	defer td.close()

	// When "auto" is used in conjunction with an attribute-expression-based test
	// pattern, dependencies should be checked.
	stdin := addGetSoftwareFeaturesResult(t, td, []string{"foo"}, []string{})
	td.cfg.Patterns = []string{"(bvt)"} // attr expr needed to check deps with "auto"
	td.cfg.checkTestDeps = checkTestDepsAuto
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkArgs(t, stdin, &runner.Args{Mode: runner.GetSoftwareFeaturesMode})
	checkRunnerTestDepsArgs(t, &td.cfg, true, []string{"foo"}, []string{})
}

func TestGetSoftwareFeaturesAutoSpecificTest(t *testing.T) {
	td := newLocalTestData()
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
	td := newLocalTestData()
	defer td.close()

	// "auto" should be downgraded to "never" if the runner didn't report knowing
	// about any features at all (probably because it's running on a non-test system image).
	stdin := addGetSoftwareFeaturesResult(t, td, []string{}, []string{})
	td.cfg.Patterns = []string{"(bvt)"} // attr expr needed to check deps with "auto"
	td.cfg.checkTestDeps = checkTestDepsAuto
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getSoftwareFeatures(%+v) failed: %v", td.cfg, err)
	}
	checkArgs(t, stdin, &runner.Args{Mode: runner.GetSoftwareFeaturesMode})
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil)
}

func TestGetSoftwareFeaturesAlwaysNoFeatures(t *testing.T) {
	td := newLocalTestData()
	defer td.close()

	// "always" should fail if the runner doesn't know about any features.
	stdin := addGetSoftwareFeaturesResult(t, td, []string{}, []string{})
	td.cfg.checkTestDeps = checkTestDepsAlways
	if err := getSoftwareFeatures(context.Background(), &td.cfg); err == nil {
		t.Fatalf("getSoftwareFeatures(%+v) succeeded unexpectedly", td.cfg)
	}
	checkArgs(t, stdin, &runner.Args{Mode: runner.GetSoftwareFeaturesMode})
}
