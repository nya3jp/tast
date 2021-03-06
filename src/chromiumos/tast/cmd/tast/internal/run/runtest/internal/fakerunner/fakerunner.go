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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/run/runtest/internal/fakesshserver"
	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
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
		fakesshserver.ExactMatchHandler(fmt.Sprintf("exec env %s -rpc", path), r.RunGRPC),
	}
}

// Install creates a fake executable at path.
// Call this method to install this as a fake remote test runner.
func (r *Runner) Install(path string) (*fakeexec.Loopback, error) {
	return fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		if len(args) == 1 {
			return r.RunJSON(stdin, stdout, stderr)
		}
		if len(args) == 2 && args[1] == "-rpc" {
			return r.RunGRPC(stdin, stdout, stderr)
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

// RunGRPC executes the fake test runner logic in gRPC protocol mode.
func (r *Runner) RunGRPC(stdin io.Reader, stdout, stderr io.Writer) int {
	// Start a local gRPC server.
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runner.Run([]string{"-rpc"}, sr, sw, ioutil.Discard, &jsonprotocol.RunnerArgs{}, r.cfg.StaticConfig)
	}()
	defer func() {
		cw.Close()
		cr.Close()
		<-done
	}()

	conn, err := rpc.NewClient(context.Background(), cr, cw, &protocol.HandshakeRequest{
		RunnerInitParams: &protocol.RunnerInitParams{
			BundleGlob: filepath.Join(r.cfg.BundleDir, "*"),
		},
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: Failed to create a local gRPC server connection: %v\n", err)
		return 1
	}
	defer conn.Close()

	rpc.RunServer(stdin, stdout, nil, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		protocol.RegisterTestServiceServer(srv, newTestService(r.cfg, protocol.NewTestServiceClient(conn.Conn())))
		return nil
	})
	return 0
}

type testService struct {
	protocol.UnimplementedTestServiceServer
	cfg *Config
	cl  protocol.TestServiceClient
}

func newTestService(cfg *Config, cl protocol.TestServiceClient) *testService {
	return &testService{cfg: cfg, cl: cl}
}

func (s *testService) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	return s.cl.ListEntities(ctx, req)
}

func (s *testService) RunTests(downstream protocol.TestService_RunTestsServer) error {
	initReq, err := downstream.Recv()
	if err != nil {
		return err
	}
	s.cfg.OnRunTestsInit(initReq.GetRunTestsInit())

	upstream, err := s.cl.RunTests(downstream.Context())
	if err != nil {
		return err
	}
	defer upstream.CloseSend()

	if err := upstream.Send(initReq); err != nil {
		return err
	}

	// Downstream -> Upstream
	go func() {
		for {
			msg, err := downstream.Recv()
			if err != nil {
				return
			}
			if err := upstream.Send(msg); err != nil {
				return
			}
		}
	}()

	// Upstream -> Downstream
	done := make(chan error)
	go func() {
		done <- func() error {
			for {
				msg, err := upstream.Recv()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				if err := downstream.Send(msg); err != nil {
					return err
				}
			}
		}()
	}()

	return <-done
}

func (s *testService) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	return s.cfg.GetDUTInfo(req)
}

func (s *testService) GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
	return s.cfg.GetSysInfoState(req)
}

func (s *testService) CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
	return s.cfg.CollectSysInfo(req)
}

func (s *testService) DownloadPrivateBundles(ctx context.Context, req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
	return s.cfg.DownloadPrivateBundles(req)
}
