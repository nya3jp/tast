// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/rpc"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"

	"google.golang.org/grpc"
)

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *Config, lc *grpc.ClientConn, rc *grpc.ClientConn) ([]*rpc.TestInfo, error) {
	var tests []*rpc.TestInfo

	if cfg.runLocal {
		localTests, err := rpc.NewTastCoreServiceClient(lc).List(ctx, &rpc.ListRequest{
			Pattern:    cfg.Patterns,
			BundleGlob: cfg.localBundleGlob(),
		})
		if err != nil {
			return nil, err
		}
		tests = append(tests, localTests.Test...)
	}
	if cfg.runRemote {
		remoteTests, err := rpc.NewTastCoreServiceClient(rc).List(ctx, &rpc.ListRequest{
			Pattern:    cfg.Patterns,
			BundleGlob: cfg.localBundleGlob(),
		})
		if err != nil {
			return nil, err
		}
		tests = append(tests, remoteTests.Test...)
	}

	return tests, nil
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
