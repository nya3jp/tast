// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"path/filepath"
	"sort"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testcontext"
)

type testServer struct {
	protocol.UnimplementedTestServiceServer
	scfg   *StaticConfig
	params *protocol.RunnerInitParams
}

func newTestServer(scfg *StaticConfig, params *protocol.RunnerInitParams) *testServer {
	return &testServer{
		scfg:   scfg,
		params: params,
	}
}

func (s *testServer) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	var entities []*protocol.ResolvedEntity
	if err := s.forEachBundle(ctx, func(ctx context.Context, ts protocol.TestServiceClient) error {
		res, err := ts.ListEntities(ctx, req) // pass through req
		if err != nil {
			return err
		}
		entities = append(entities, res.GetEntities()...)
		return nil
	}); err != nil {
		return nil, err
	}
	return &protocol.ListEntitiesResponse{Entities: entities}, nil
}

func (s *testServer) RunTests(srv protocol.TestService_RunTestsServer) error {
	ctx := srv.Context()
	ctx = testcontext.WithLogger(ctx, func(msg string) {
		srv.Send(&protocol.RunTestsResponse{
			Type: &protocol.RunTestsResponse_RunLog{
				RunLog: &protocol.RunLogEvent{
					Time: ptypes.TimestampNow(),
					Text: msg,
				},
			},
		})
	})

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

	return s.forEachBundle(ctx, func(ctx context.Context, ts protocol.TestServiceClient) error {
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

func (s *testServer) forEachBundle(ctx context.Context, f func(ctx context.Context, ts protocol.TestServiceClient) error) error {
	bundlePaths, err := filepath.Glob(s.params.GetBundleGlob())
	if err != nil {
		return err
	}
	// Sort bundles for determinism.
	sort.Strings(bundlePaths)

	for _, bundlePath := range bundlePaths {
		if err := func() error {
			cl, err := rpc.DialExec(ctx, bundlePath, true, &protocol.HandshakeRequest{})
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
