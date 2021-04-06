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
	bundleGlob string
}

func newTestServer(bundleGlob string) *testServer {
	return &testServer{
		bundleGlob: bundleGlob,
	}
}

func (s *testServer) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	bundlePaths, err := filepath.Glob(s.bundleGlob)
	if err != nil {
		return nil, err
	}

	// Sort bundles for determinism.
	sort.Strings(bundlePaths)

	var entities []*protocol.ResolvedEntity
	for _, bundlePath := range bundlePaths {
		if err := func() error {
			cl, err := rpc.DialExec(ctx, bundlePath, &protocol.HandshakeRequest{})
			if err != nil {
				return err
			}
			defer cl.Close(ctx)

			ts := protocol.NewTestServiceClient(cl.Conn)
			res, err := ts.ListEntities(ctx, req) // pass through req
			if err != nil {
				return err
			}

			entities = append(entities, res.GetEntities()...)
			return nil
		}(); err != nil {
			return nil, errors.Wrap(err, filepath.Base(bundlePath))
		}
	}
	return &protocol.ListEntitiesResponse{Entities: entities}, nil
}
