// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"path/filepath"
	"sort"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
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
