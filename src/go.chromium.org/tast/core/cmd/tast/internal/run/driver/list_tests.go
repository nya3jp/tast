// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"

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
	if d.localRunnerClient() == nil {
		return nil, nil
	}
	tests, err := d.localRunnerClient().ListTests(ctx, d.cfg.Patterns(), features)
	if err != nil {
		if !d.healthy(ctx) {
			logging.Infof(ctx, "The connection to DUT %s is not healthy", d.rawTarget)
		} else {
			logging.Infof(ctx, "The connection to DUT %s is healthy", d.rawTarget)
		}
		return nil, errors.Wrap(err, "failed to list local tests")
	}
	return tests, nil
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
