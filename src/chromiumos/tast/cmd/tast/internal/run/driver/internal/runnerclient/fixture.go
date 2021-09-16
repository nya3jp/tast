// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"chromiumos/tast/cmd/tast/internal/run/driverdata"
	"chromiumos/tast/internal/jsonprotocol"
)

// fixtureGraph is a collection of entity graphs that cover fixtures in all
// test bundles. Note that it covers fixtures only; tests are not included
// in the graphs.
type fixtureGraph struct {
	// ascendants is a map from (bundle name, fixture name) to one of its
	// ascendant fixture name. An ascendant can be a direct or indirect
	// parent.
	ascendants map[fixtureKey]string
}

type fixtureKey struct {
	Bundle string
	Name   string
}

// FindStart finds a start fixture name of a given fixture. If it cannot find
// a fixture with the name in the entity graphs, it returns name as is.
// Since fixtureGraph covers fixtures only, it is wrong to pass a test name to
// this method.
func (g *fixtureGraph) FindStart(bundle, name string) string {
	for {
		asc, ok := g.ascendants[fixtureKey{Bundle: bundle, Name: name}]
		if !ok {
			return name
		}
		name = asc
	}
}

// newFixtureGraphFromBundleEntities constructs fixtureGraph from
// BundleEntity.
func newFixtureGraphFromBundleEntities(fixtures []*driverdata.BundleEntity) *fixtureGraph {
	starts := make(map[fixtureKey]string)
	for _, f := range fixtures {
		key := fixtureKey{
			Bundle: f.Bundle,
			Name:   f.Resolved.GetEntity().GetName(),
		}
		starts[key] = f.Resolved.GetStartFixtureName()
	}
	return &fixtureGraph{ascendants: starts}
}

// newFixtureGraphFromListFixturesResult constructs fixtureGraph from
// RunnerListFixturesResult.
func newFixtureGraphFromListFixturesResult(res *jsonprotocol.RunnerListFixturesResult) *fixtureGraph {
	parents := make(map[fixtureKey]string)
	for bundle, fs := range res.Fixtures {
		for _, f := range fs {
			key := fixtureKey{
				Bundle: bundle,
				Name:   f.Name,
			}
			parents[key] = f.Fixture
		}
	}
	return &fixtureGraph{ascendants: parents}
}
