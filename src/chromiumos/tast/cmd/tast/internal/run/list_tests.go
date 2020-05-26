// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"log"

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/rpc"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
)

type bundleType int

const (
	local bundleType = iota
	remote
)

func (b bundleType) String() string {
	switch b {
	case local:
		return "local"
	case remote:
		return "remote"
	default:
		log.Panicf("Unknown bundleType %d", b)
	}
	return ""
}

type allTests map[bundleType]map[string]*rpc.ListResponse // type -> bundle -> info

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *Config, lc rpc.TastCoreServiceClient, rc rpc.TastCoreServiceClient) (allTests, error) {
	res := map[bundleType]map[string]*rpc.ListResponse{local: {}, remote: {}}

	if cfg.runLocal {
		log.Println("listTests -> localTests.List")
		bs, err := lc.Bundles(ctx, &rpc.BundlesRequest{BundleGlob: cfg.localBundleGlob()})
		if err != nil {
			return nil, err
		}

		for _, b := range bs.Bundles {
			localTests, err := lc.List(ctx, &rpc.ListRequest{
				Pattern: cfg.Patterns,
				Bundle:  b,
			})
			if err != nil {
				return nil, err
			}
			res[local][b] = localTests
		}
	}
	if cfg.runRemote {
		log.Println("listTests -> remoteTests.List")
		bs, err := rc.Bundles(ctx, &rpc.BundlesRequest{BundleGlob: cfg.remoteBundleGlob()})
		if err != nil {
			log.Fatal("remote Bundles failed:", err)
			return nil, err
		}

		log.Print("got remote tests: ", bs)

		for _, b := range bs.Bundles {
			localTests, err := rc.List(ctx, &rpc.ListRequest{
				Pattern: cfg.Patterns,
				Bundle:  b,
			})
			if err != nil {
				log.Fatal("remote List failed2:", err)
				return nil, err
			}
			res[remote][b] = localTests
		}
	}

	return res, nil
}

// listLocalTests returns a list of local tests to run.
func listLocalTests(ctx context.Context, cfg *Config, hst *ssh.Conn) ([]testing.TestInstance, error) {
	return runListTestsCommand(
		localRunnerCommand(ctx, cfg, hst), cfg.Patterns, cfg.localBundleGlob())
}

// listRemoteTests returns a list of remote tests to run.
func listRemoteTests(ctx context.Context, cfg *Config) ([]testing.TestInstance, error) {
	return runListTestsCommand(
		remoteRunnerCommand(ctx, cfg), cfg.Patterns, cfg.remoteBundleGlob())
}

func runListTestsCommand(r runnerCmd, ptns []string, glob string) ([]testing.TestInstance, error) {
	var ts []testing.TestInstance
	if err := runTestRunnerCommand(
		r,
		&runner.Args{
			Mode: runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{
				BundleArgs: bundle.ListTestsArgs{Patterns: ptns},
				BundleGlob: glob,
			},
		},
		&ts,
	); err != nil {
		return nil, err
	}
	return ts, nil
}
