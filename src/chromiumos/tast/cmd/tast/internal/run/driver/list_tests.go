// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// ListMatchedTests enumerates tests matched with the user-supplied patterns.
func (d *Driver) ListMatchedTests(ctx context.Context, features *protocol.Features) ([]*protocol.ResolvedEntity, error) {
	local, err := d.localClient().ListTests(ctx, d.cfg.Patterns(), features)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list local tests")
	}
	remote, err := d.remoteClient().ListTests(ctx, d.cfg.Patterns(), features)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list remote tests")
	}
	return append(local, remote...), nil
}

// ListMatchedLocalTests enumerates local tests matched with the user-supplied
// patterns.
func (d *Driver) ListMatchedLocalTests(ctx context.Context, features *protocol.Features) ([]*protocol.ResolvedEntity, error) {
	tests, err := d.localClient().ListTests(ctx, d.cfg.Patterns(), features)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list local tests")
	}
	return tests, nil
}

// ListLocalFixtures enumerates all local fixtures.
func (d *Driver) ListLocalFixtures(ctx context.Context) ([]*protocol.ResolvedEntity, error) {
	fixtures, err := d.localClient().ListFixtures(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list local fixtures")
	}
	return fixtures, nil
}

// ListRemoteFixtures enumerates all remote fixtures.
func (d *Driver) ListRemoteFixtures(ctx context.Context) ([]*protocol.ResolvedEntity, error) {
	fixtures, err := d.remoteClient().ListFixtures(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list remote fixtures")
	}
	return fixtures, nil
}
