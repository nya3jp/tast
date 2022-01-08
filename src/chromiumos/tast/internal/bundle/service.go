// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle/bundleclient"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

type testServer struct {
	protocol.UnimplementedTestServiceServer
	scfg         *StaticConfig
	bundleParams *protocol.BundleInitParams
}

func newTestServer(scfg *StaticConfig, bundleParams *protocol.BundleInitParams) *testServer {
	exec.Command("logger", "New test server is setup in bundle to listen to requests").Run()
	return &testServer{scfg: scfg, bundleParams: bundleParams}
}

func (s *testServer) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	var entities []*protocol.ResolvedEntity
	// Logging added for b/213616631 to see ListEntities progress on the DUT.
	execName, err := os.Executable()
	if err != nil {
		execName = "bundle"
	}
	logging.Debugf(ctx, "Serving ListEntities Request in %s (recursive flag: %v)", execName, req.GetRecursive())
	exec.Command("logger", fmt.Sprintf("Serving ListEntities Request in %s", execName)).Run()
	if req.GetRecursive() {
		var cl *bundleclient.Client
		if s.bundleParams.GetBundleConfig() != nil {
			var err error
			cl, err = bundleclient.New(ctx, s.bundleParams.GetBundleConfig().GetPrimaryTarget(), s.scfg.registry.Name(), &protocol.HandshakeRequest{})
			if err != nil {
				return nil, err
			}
			defer cl.Close(ctx)
		}

		var err error
		entities, err = listEntitiesRecursive(ctx, s.scfg.registry, req.Features, cl)
		if err != nil {
			return nil, err
		}
	} else {
		entities = listEntities(s.scfg.registry, req.Features)
	}
	// Logging added for b/213616631 to see ListEntities progress on the DUT.
	logging.Debugf(ctx, "Successfully serving ListEntities Request in %s ", execName)
	exec.Command("logger", fmt.Sprintf("Successfully serving ListEntities Request in %s", execName)).Run()
	return &protocol.ListEntitiesResponse{Entities: entities}, nil
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

	if initReq.GetRunTestsInit().GetRecursive() {
		return runTestsRecursive(ctx, srv, initReq.GetRunTestsInit().GetRunConfig(), s.scfg, s.bundleParams)
	}
	return runTests(ctx, srv, initReq.GetRunTestsInit().GetRunConfig(), s.scfg, s.bundleParams.GetBundleConfig())
}

// listEntitiesRecursive lists all the entities this bundle has.
// If cl is non-nil it also lists all the entities in the bundle cl points to.
func listEntitiesRecursive(ctx context.Context, reg *testing.Registry, features *protocol.Features, cl *bundleclient.Client) ([]*protocol.ResolvedEntity, error) {
	entities := listEntities(reg, features)
	if cl == nil {
		return entities, nil
	}
	es, err := cl.TestService().ListEntities(ctx, &protocol.ListEntitiesRequest{
		Features:  features,
		Recursive: true,
	})
	if err != nil {
		return nil, err
	}

	for _, e := range es.Entities {
		e.Hops++
		entities = append(entities, e)
	}
	return entities, nil
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
