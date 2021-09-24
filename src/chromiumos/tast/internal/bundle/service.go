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
	bcfg *protocol.BundleConfig
}

func newTestServer(scfg *StaticConfig, bcfg *protocol.BundleConfig) *testServer {
	return &testServer{scfg: scfg, bcfg: bcfg}
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

	return runTests(ctx, srv, initReq.GetRunTestsInit().GetRunConfig(), s.scfg, s.bcfg)
}

func listEntities(reg *testing.Registry, features *protocol.Features) []*protocol.ResolvedEntity {
	fixtures := reg.AllFixtures()
	starts := buildStartFixtureMap(fixtures)

	var resolved []*protocol.ResolvedEntity

	for _, f := range fixtures {
		resolved = append(resolved, &protocol.ResolvedEntity{
			Entity:           f.EntityProto(),
			StartFixtureName: starts[f.Name],
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
		start, ok := starts[t.Fixture]
		if !ok {
			start = t.Fixture
		}
		resolved = append(resolved, &protocol.ResolvedEntity{
			Entity:           t.EntityProto(),
			Skip:             skip,
			StartFixtureName: start,
		})
	}
	return resolved
}

func buildStartFixtureMap(fixtures map[string]*testing.FixtureInstance) map[string]string {
	starts := make(map[string]string)

	// findStart is a recursive function to find a start fixture of f.
	// It fills in results to starts for memoization.
	var findStart func(f *testing.FixtureInstance) string
	findStart = func(f *testing.FixtureInstance) string {
		if start, ok := starts[f.Name]; ok {
			return start // memoize
		}
		var start string
		if parent, ok := fixtures[f.Parent]; ok {
			start = findStart(parent)
		} else {
			start = f.Parent
		}
		starts[f.Name] = start
		return start
	}

	for _, f := range fixtures {
		findStart(f)
	}
	return starts
}
