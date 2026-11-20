// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"
	"slices"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
)

// ListMatchedTests enumerates tests matched with the user-supplied patterns.
func (d *Driver) ListMatchedTests(ctx context.Context, features *protocol.Features) ([]*BundleEntity, error) {
	local, err := d.ListMatchedLocalTests(ctx, features)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list local tests")
	}
	remote, err := d.remoteRunnerClient().ListTests(ctx, d.cfg.Patterns(), features)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list remote tests")
	}
	return append(local, remote...), nil
}

// ListMatchedLocalTests enumerates local tests matched with the user-supplied
// patterns.
func (d *Driver) ListMatchedLocalTests(ctx context.Context, features *protocol.Features) ([]*BundleEntity, error) {
	tests, _, err := d.ListMatchedLocalTestsAndFixtures(ctx, features)
	return tests, err
}

// ListMatchedLocalTestsAndFixtures enumerates local tests matched with the
// user-supplied patterns and all local fixtures with a single request to the
// DUT. Later calls with the same features reuse the result.
func (d *Driver) ListMatchedLocalTestsAndFixtures(ctx context.Context, features *protocol.Features) (tests, fixtures []*BundleEntity, err error) {
	if d.localRunnerClient() == nil {
		return nil, nil, nil
	}
	// Listing starts the test runner and the bundles on the DUT, and preparing
	// the DUT lists entities with the same features as running tests.
	// d.cfg.Patterns() is fixed for d, so the features are the key.
	if l := d.localEntities; l != nil && proto.Equal(l.features, features) {
		return slices.Clone(l.tests), slices.Clone(l.fixtures), nil
	}
	tests, fixtures, err = d.localRunnerClient().ListTestsAndFixtures(ctx, d.cfg.Patterns(), features)
	if err != nil {
		if !d.healthy(ctx) {
			logging.Infof(ctx, "The connection to DUT %s is not healthy", d.rawTarget)
		} else {
			logging.Infof(ctx, "The connection to DUT %s is healthy", d.rawTarget)
		}
		return nil, nil, errors.Wrap(err, "failed to list local tests")
	}
	// Keep copies, as callers may modify features and the returned slices.
	d.localEntities = &localEntityList{
		features: proto.Clone(features).(*protocol.Features),
		tests:    slices.Clone(tests),
		fixtures: slices.Clone(fixtures),
	}
	return tests, fixtures, nil
}

// localEntityList is the result of listing local entities with features.
type localEntityList struct {
	features        *protocol.Features
	tests, fixtures []*BundleEntity
}

func (d *Driver) healthy(ctx context.Context) bool {
	if d.cc == nil || d.cc.Conn() == nil {
		return false
	}
	if err := d.cc.Conn().Healthy(ctx); err != nil {
		return false
	}
	return true
}

// ListLocalFixtures enumerates all local fixtures.
func (d *Driver) ListLocalFixtures(ctx context.Context) ([]*BundleEntity, error) {
	if d.localRunnerClient() == nil {
		return nil, nil
	}
	fixtures, err := d.localRunnerClient().ListFixtures(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list local fixtures")
	}
	return fixtures, nil
}

// ListRemoteFixtures enumerates all remote fixtures.
func (d *Driver) ListRemoteFixtures(ctx context.Context) ([]*BundleEntity, error) {
	fixtures, err := d.remoteRunnerClient().ListFixtures(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list remote fixtures")
	}
	return fixtures, nil
}
