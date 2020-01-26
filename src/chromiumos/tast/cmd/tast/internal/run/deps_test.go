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
	"chromiumos/tast/internal/runner"
)

// writeGetDUTInfoResult writes runner.GetDUTInfoResult to w.
func writeGetDUTInfoResult(w io.Writer, avail, unavail []string) error {
	res := runner.GetDUTInfoResult{
		SoftwareFeatures: &runner.SoftwareFeatures{
			Available:   avail,
			Unavailable: unavail,
		},
	}
	return json.NewEncoder(w).Encode(&res)
}

// checkRunnerTestDepsArgs calls setRunnerTestDepsArgs using cfg and verifies
// that it sets runner args as specified per checkDeps, avail, and unavail.
func checkRunnerTestDepsArgs(t *testing.T, cfg *Config, checkDeps bool, avail, unavail []string) {
	t.Helper()
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

func TestGetDUTInfo(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// With "always", features returned by the runner should be passed through
	// and dependencies should be checked.
	avail := []string{"dep1", "dep2"}
	unavail := []string{"dep3"}
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.GetDUTInfoMode,
			GetDUTInfo: &runner.GetDUTInfoArgs{
				ExtraUSEFlags: td.cfg.extraUSEFlags,
			},
		})

		writeGetDUTInfoResult(stdout, avail, unavail)
		return 0
	}
	td.cfg.checkTestDeps = true
	td.cfg.extraUSEFlags = []string{"use1", "use2"}
	if err := getDUTInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getDUTInfo(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, true, avail, unavail)

	// The second call should fail, because it tries to update cfg's fields twice.
	if err := getDUTInfo(context.Background(), &td.cfg); err == nil {
		t.Fatal("Calling getDUTInfo twice unexpectedly succeeded")
	}
}

func TestGetDUTInfoNoCheckTestDeps(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	// With "never", the runner shouldn't be called and dependencies shouldn't be checked.
	td.cfg.checkTestDeps = false
	if err := getDUTInfo(context.Background(), &td.cfg); err != nil {
		t.Fatalf("getDUTInfo(%+v) failed: %v", td.cfg, err)
	}
	checkRunnerTestDepsArgs(t, &td.cfg, false, nil, nil)
}

func TestGetSoftwareFeaturesNoFeatures(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()
	// "always" should fail if the runner doesn't know about any features.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:       runner.GetDUTInfoMode,
			GetDUTInfo: &runner.GetDUTInfoArgs{},
		})
		writeGetDUTInfoResult(stdout, []string{}, []string{})
		return 0
	}
	td.cfg.checkTestDeps = true
	if err := getDUTInfo(context.Background(), &td.cfg); err == nil {
		t.Fatalf("getSoftwareFeatures(%+v) succeeded unexpectedly", td.cfg)
	}
}
