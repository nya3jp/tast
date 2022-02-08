// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
)

type testServer struct {
	protocol.UnimplementedTestServiceServer
	scfg         *StaticConfig
	runnerParams *protocol.RunnerInitParams
	bundleParams *protocol.BundleInitParams
}

func newTestServer(scfg *StaticConfig, runnerParams *protocol.RunnerInitParams, bundleParams *protocol.BundleInitParams) *testServer {
	exec.Command("logger", "local_test_runner: New test server is up for serving requests").Run()
	return &testServer{
		scfg:         scfg,
		runnerParams: runnerParams,
		bundleParams: bundleParams,
	}
}

func (s *testServer) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving GetDUTInfo Request")
	exec.Command("logger", "local_test_runner: Serving GetDUTInfo Request").Run()
	if s.scfg.GetDUTInfo == nil {
		return &protocol.GetDUTInfoResponse{}, nil
	}
	return s.scfg.GetDUTInfo(ctx, req)
}

func (s *testServer) GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving GetSysInfoState Request")
	exec.Command("logger", "local_test_runner: Serving GetSysInfoState Request").Run()
	if s.scfg.GetSysInfoState == nil {
		return &protocol.GetSysInfoStateResponse{}, nil
	}
	return s.scfg.GetSysInfoState(ctx, req)
}

func (s *testServer) CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving CollectSysInfo Request")
	exec.Command("logger", "local_test_runner: Serving CollectSysInfo Request").Run()
	if s.scfg.CollectSysInfo == nil {
		return &protocol.CollectSysInfoResponse{}, nil
	}
	return s.scfg.CollectSysInfo(ctx, req)
}

func (s *testServer) DownloadPrivateBundles(ctx context.Context, req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving DownloadPrivateBundles Request")
	exec.Command("logger", "local_test_runner: Serving DownloadPrivateBundles Request").Run()

	if s.scfg.PrivateBundlesStampPath == "" {
		return nil, errors.New("this test runner is not configured for private bundles")
	}

	if req.GetBuildArtifactUrl() == "" {
		return nil, errors.New("failed to determine the build artifacts URL (non-official image?)")
	}

	// If the stamp file exists, private bundles have been already downloaded.
	if _, err := os.Stat(s.scfg.PrivateBundlesStampPath); err == nil {
		return &protocol.DownloadPrivateBundlesResponse{}, nil
	}

	// Download the archive via devserver.
	archiveURL := req.GetBuildArtifactUrl() + "tast_bundles.tar.bz2"
	logging.Infof(ctx, "Downloading private bundles from %s", archiveURL)
	cl, err := devserver.NewClient(
		ctx, req.GetServiceConfig().GetDevservers(),
		req.GetServiceConfig().GetTlwServer(), req.GetServiceConfig().GetTlwSelfName(),
		req.ServiceConfig.GetDutServer(),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new client [devservers=%v, TLWServer=%s]",
			req.GetServiceConfig().GetDevservers(), req.GetServiceConfig().GetTlwServer())
	}
	defer cl.TearDown()

	r, err := cl.Open(ctx, archiveURL)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	tf, err := ioutil.TempFile("", "tast_bundles.")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tf.Name())

	_, err = io.Copy(tf, r)

	if cerr := tf.Close(); err == nil {
		err = cerr
	}

	if err == nil {
		// Extract the archive, and touch the stamp file.
		cmd := exec.Command("tar", "xf", tf.Name())
		cmd.Dir = "/usr/local"
		if err := cmd.Run(); err != nil {
			return nil, errors.Errorf("failed to extract %s: %v", strings.Join(cmd.Args, " "), err)
		}
		logging.Info(ctx, "Download finished successfully")
	} else if os.IsNotExist(err) {
		logging.Info(ctx, "Private bundles not found")
	} else {
		return nil, errors.Errorf("failed to download %s: %v", archiveURL, err)
	}

	if err := ioutil.WriteFile(s.scfg.PrivateBundlesStampPath, nil, 0644); err != nil {
		return nil, err
	}

	return &protocol.DownloadPrivateBundlesResponse{}, nil
}

func (s *testServer) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	var entities []*protocol.ResolvedEntity
	// Logging added for b/213616631 to see ListEntities progress on the DUT.
	logging.Debug(ctx, "Serving ListEntities Request")
	exec.Command("logger", "local_test_runner: Serving ListEntities Request").Run()
	// ListEntities should not set runtime global information during handshake.
	// TODO(b/187793617): Always pass s.bundleParams to bundles once we fully migrate to gRPC-based protocol.
	// This workaround is currently needed because BundleInitParams is unavailable when this method is called internally for handling JSON-based protocol methods.
	if err := s.forEachBundle(ctx, nil, func(ctx context.Context, ts protocol.TestServiceClient) error {
		res, err := ts.ListEntities(ctx, req) // pass through req
		if err != nil {
			return err
		}
		entities = append(entities, res.GetEntities()...)
		return nil
	}); err != nil {
		return nil, err
	}
	// Logging added for b/213616631 to see ListEntities progress on the DUT.
	logging.Debug(ctx, "Finish serving ListEntities Request")
	exec.Command("logger", "local_test_runner: Finish serving ListEntities Request").Run()
	return &protocol.ListEntitiesResponse{Entities: entities}, nil
}

func (s *testServer) RunTests(srv protocol.TestService_RunTestsServer) error {
	// Logging added for b/213616631.
	exec.Command("logger", "local_test_runner: Serving RunTests Request").Run()
	ctx := srv.Context()
	logger := logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(func(msg string) {
		srv.Send(&protocol.RunTestsResponse{
			Type: &protocol.RunTestsResponse_RunLog{
				RunLog: &protocol.RunLogEvent{
					Time: ptypes.TimestampNow(),
					Text: msg,
				},
			},
		})
	}))
	// Logs from RunTests should not be routed to protocol.Logging service.
	ctx = logging.AttachLoggerNoPropagation(ctx, logger)

	initReq, err := srv.Recv()
	if err != nil {
		return err
	}
	if _, ok := initReq.GetType().(*protocol.RunTestsRequest_RunTestsInit); !ok {
		return errors.Errorf("RunTests: unexpected initial request message: got %T, want %T", initReq.GetType(), &protocol.RunTestsRequest_RunTestsInit{})
	}

	if s.scfg.KillStaleRunners {
		killStaleRunners(ctx, unix.SIGTERM)
	}

	return s.forEachBundle(ctx, s.bundleParams, func(ctx context.Context, ts protocol.TestServiceClient) error {
		st, err := ts.RunTests(ctx)
		if err != nil {
			return err
		}
		defer st.CloseSend()

		// Duplicate the initial request.
		if err := st.Send(initReq); err != nil {
			return err
		}

		// Relay responses.
		for {
			res, err := st.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := srv.Send(res); err != nil {
				return err
			}
		}
	})
}

func (s *testServer) forEachBundle(ctx context.Context, bundleParams *protocol.BundleInitParams, f func(ctx context.Context, ts protocol.TestServiceClient) error) error {
	bundlePaths, err := filepath.Glob(s.runnerParams.GetBundleGlob())
	if err != nil {
		return err
	}
	// Sort bundles for determinism.
	sort.Strings(bundlePaths)

	for _, bundlePath := range bundlePaths {
		if err := func() error {
			// Logging added for b/213616631 to see ListEntities progress on the DUT.
			logging.Debugf(ctx, "Sending request to bundle %s", bundlePath)
			cl, err := rpc.DialExec(ctx, bundlePath, true,
				&protocol.HandshakeRequest{BundleInitParams: bundleParams})
			if err != nil {
				return err
			}
			defer cl.Close()

			return f(ctx, protocol.NewTestServiceClient(cl.Conn()))
		}(); err != nil {
			return errors.Wrap(err, filepath.Base(bundlePath))
		}
	}
	return nil
}
