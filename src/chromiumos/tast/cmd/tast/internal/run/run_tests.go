// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"

	"chromiumos/tast/errors"
)

func runTests(ctx context.Context, cfg *Config) ([]EntityResult, error) {
	if err := getDUTInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}

	if cfg.osVersion == "" {
		cfg.Logger.Log("Target version: not available from target")
	} else {
		cfg.Logger.Logf("Target version: %v", cfg.osVersion)
	}

	if err := getInitialSysInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}
	cfg.startedRun = true

	var results []EntityResult
	if cfg.runLocal {
		lres, err := runLocalTests(ctx, cfg)
		results = append(results, lres...)
		if err != nil {
			// TODO(derat): While test runners are always supposed to report success even if tests fail,
			// it'd probably be better to run both types here even if one fails.
			return results, err
		}
	}

	// Turn down the ephemeral devserver before running remote tests. Some remote tests
	// in the meta category run the tast command which starts yet another ephemeral devserver
	// and reverse forwarding port can conflict.
	closeEphemeralDevserver(ctx, cfg)

	// Run remote tests and merge the results.
	if !cfg.runRemote {
		return results, nil
	}

	rres, err := runRemoteTests(ctx, cfg)
	results = append(results, rres...)
	return results, err
}
