// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

type testServer struct {
	protocol.UnimplementedTestServiceServer
	scfg *StaticConfig
}

func newTestServer(scfg *StaticConfig) *testServer {
	return &testServer{scfg: scfg}
}

func (s *testServer) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	resolved := listEntities(s.scfg.registry, req.GetFeatures())
	return &protocol.ListEntitiesResponse{Entities: resolved}, nil
}

func (s *testServer) RunTests(srv protocol.TestService_RunTestsServer) error {
	ctx := srv.Context()

	initReq, err := srv.Recv()
	if err != nil {
		return err
	}
	if _, ok := initReq.GetType().(*protocol.RunTestsRequest_RunTestsInit); !ok {
		return errors.Errorf("RunTests: unexpected initial request message: got %T, want %T", initReq.GetType(), &protocol.RunTestsRequest_RunTestsInit{})
	}
	init := initReq.GetRunTestsInit()

	return runTests(ctx, srv, init.GetRunConfig(), s.scfg)
}

func listEntities(reg *testing.Registry, features *protocol.Features) []*protocol.ResolvedEntity {
	var resolved []*protocol.ResolvedEntity

	for _, f := range reg.AllFixtures() {
		resolved = append(resolved, &protocol.ResolvedEntity{
			Entity: f.EntityProto(),
		})
	}

	for _, t := range reg.AllTests() {
		// If we encounter errors while checking test dependencies,
		// treat the test as not skipped. When we actually try to
		// run the test later, it will fail with errors.
		var skip *protocol.Skip
		if reasons, err := t.Deps().Check(features); err == nil && len(reasons) > 0 {
			skip = &protocol.Skip{Reasons: reasons}
		}
		resolved = append(resolved, &protocol.ResolvedEntity{
			Entity: t.EntityProto(),
			Skip:   skip,
		})
	}
	return resolved
}
