// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"chromiumos/tast/errors"
	"chromiumos/tast/rpc"
)

func runTestsV2(ctx context.Context, cfg *Config, lc rpc.TastCoreServiceClient, rc rpc.TastCoreServiceClient) ([]TestResult, error) {
	log.Print("tast: runTestsV2")
	if err := getDUTInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}
	if err := getInitialSysInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}
	cfg.startedRun = true

	ts, err := listTests(ctx, cfg, lc, rc) // local? -> bundle -> tests
	if err != nil {
		return nil, fmt.Errorf("Failed to list tests: %v", err)
	}

	if n := len(ts[remote]); n != 1 {
		log.Panicf("Got %d remote bundles, want 1", n)
	}
	var remoteBundle string
	remotePre := make(map[string]string)
	for k, ls := range ts[remote] {
		remoteBundle = k
		for _, p := range ls.Preconditions {
			remotePre[p.Name] = p.Parent
		}
	}

	//////////////////////////////// Run local tests //////////////////////////////////
	if cfg.runLocal {
		for bundle, ls := range ts[local] {
			pre := make(map[string]string)
			for _, p := range ls.Preconditions {
				if _, ok := remotePre[p.Name]; ok {
					return nil, fmt.Errorf("multiply defined precondition %s in %s-%s and %s-%s", p.Name, local,
						filepath.Base(bundle), remote, filepath.Base(remoteBundle),
					)
				}
				pre[p.Name] = p.Parent
			}

			if err != nil {
				return nil, fmt.Errorf("runLocalTestsV2 failed: %v", err)
			}
		}
	}
	// if cfg.runLocal {
	// 	lres, err := runLocalTests(ctx, cfg)
	// 	results = append(results, lres...)
	// 	if err != nil {
	// 		// TODO(derat): While test runners are always supposed to report success even if tests fail,
	// 		// it'd probably be better to run both types here even if one fails.
	// 		return results, err
	// 	}
	// }

	// Turn down the ephemeral devserver before running remote tests. Some remote tests
	// tegory run the tast command which starts yet another ephemeral devserver
	// and reverse forwarding port can conflict.
	closeEphemeralDevserver(ctx, cfg)

	//////////////////////////////// Done local tests //////////////////////////////////

	// Run remote tests and merge the results.
	if !cfg.runRemote {
	}

	// rres, err := runRemoteTests(ctx, cfg)
	// results = append(results, rres...)
	// return results, err
	panic("hoge")
	return nil, nil
}

func runTests(ctx context.Context, cfg *Config) ([]TestResult, error) {
	log.Print("tast: runTests")
	if err := getDUTInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}
	if err := getInitialSysInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}
	cfg.startedRun = true

	var results []TestResult
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
