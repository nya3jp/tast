// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakerunner provides a fake implementation of test runners.
//
// We implement fake test runners by mostly delegating to the real
// implementation of test runners (chromiumos/tast/internal/runner). Thus we can
// use the fakebundle package to create fake test bundles executed by fake test
// runners. On the other hand, other operation modes (e.g. GetDUTInfo,
// GetSysInfoState etc.) are hooked by this package so that callers can inject
// custom implementations of them.
package fakerunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"chromiumos/tast/cmd/tast/internal/run/runtest/internal/fakesshserver"
	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/shutil"
)

// Config holds configuration values needed to instantiate a fake test runner.
type Config struct {
	// BundleDir is a path of the directory containing test bundles.
	BundleDir string
	// StaticConfig is a configuration passed to the test runner.
	StaticConfig *runner.StaticConfig
	// GetDUTInfo implements the GetDUTInfo handler.
	GetDUTInfo func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)
	// GetSysInfoState implements the GetSysInfoState handler.
	GetSysInfoState func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error)
	// CollectSysInfo implements the CollectSysInfo handler.
	CollectSysInfo func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error)
	// DownloadPrivateBundles implements the DownloadPrivateBundles handler.
	DownloadPrivateBundles func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error)
	// OnRunTestsInit is called on the beginning of RunTests.
	OnRunTestsInit func(init *protocol.RunTestsInit)
}

// Runner represents a fake test runner.
type Runner struct {
	cfg *Config
}

// New instantiates a new fake test runner from cfg.
//
// This function does not install the created fake test runner. Call SSHHandlers
// or Install to actually install it.
//
// Note: This function does not use the functional option pattern. This package
// is internal to the runtest package so it is easy to rewrite all callers when
// we introduce new configuration values.
func New(cfg *Config) *Runner {
	return &Runner{cfg: cfg}
}

// SSHHandlers returns a handler to be installed to a fake SSH server.
// Use this method to install this as a fake local test runner.
func (r *Runner) SSHHandlers(path string) []fakesshserver.Handler {
	return []fakesshserver.Handler{
		fakesshserver.ExactMatchHandler(fmt.Sprintf("exec env %s", path), r.RunJSON),
	}
}

// Install creates a fake executable at path.
// Call this method to install this as a fake remote test runner.
func (r *Runner) Install(path string) (*fakeexec.Loopback, error) {
	return fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		if len(args) == 1 {
			return r.RunJSON(stdin, stdout, stderr)
		}
		fmt.Fprintf(stderr, "ERROR: Unknown arguments: %s\n", shutil.EscapeSlice(args))
		return 1
	})
}

// RunJSON executes the fake test runner logic in JSON protocol mode.
func (r *Runner) RunJSON(stdin io.Reader, stdout, stderr io.Writer) int {
	var args jsonprotocol.RunnerArgs
	if err := json.NewDecoder(stdin).Decode(&args); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}

	switch args.Mode {
	case jsonprotocol.RunnerRunTestsMode:
		r.cfg.OnRunTestsInit(&protocol.RunTestsInit{
			RunConfig: args.RunTests.BundleArgs.Proto(),
		})
		fallthrough
	case jsonprotocol.RunnerListTestsMode, jsonprotocol.RunnerListFixturesMode:
		argsData, _ := json.Marshal(&args) // should always succeed
		return runner.Run(nil, bytes.NewBuffer(argsData), stdout, stderr, &args, r.cfg.StaticConfig)
	case jsonprotocol.RunnerGetDUTInfoMode:
		req := args.GetDUTInfo.Proto()
		res, err := r.cfg.GetDUTInfo(req)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: GetDUTInfo: %v\n", err)
			return 1
		}
		json.NewEncoder(stdout).Encode(jsonprotocol.RunnerGetDUTInfoResultFromProto(res))
		return 0
	case jsonprotocol.RunnerGetSysInfoStateMode:
		req := &protocol.GetSysInfoStateRequest{}
		res, err := r.cfg.GetSysInfoState(req)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: GetSysInfoState: %v\n", err)
			return 1
		}
		json.NewEncoder(stdout).Encode(jsonprotocol.RunnerGetSysInfoStateResultFromProto(res))
		return 0
	case jsonprotocol.RunnerCollectSysInfoMode:
		req := args.CollectSysInfo.Proto()
		res, err := r.cfg.CollectSysInfo(req)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: CollectSysInfo: %v\n", err)
			return 1
		}
		json.NewEncoder(stdout).Encode(jsonprotocol.RunnerCollectSysInfoResultFromProto(res))
		return 0
	case jsonprotocol.RunnerDownloadPrivateBundlesMode:
		req := args.DownloadPrivateBundles.Proto()
		res, err := r.cfg.DownloadPrivateBundles(req)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: DownloadPrivateBundles: %v\n", err)
			return 1
		}
		json.NewEncoder(stdout).Encode(jsonprotocol.RunnerDownloadPrivateBundlesResultFromProto(res))
		return 0
	default:
		fmt.Fprintf(stderr, "ERROR: Not implemented mode: %v\n", args.Mode)
		return 1
	}
}
