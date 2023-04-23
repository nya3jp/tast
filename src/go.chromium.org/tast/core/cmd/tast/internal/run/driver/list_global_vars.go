// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"
	"sort"

	"go.chromium.org/tast/core/errors"
)

// GlobalRuntimeVars in driver class append result between local and remote.
func (d *Driver) GlobalRuntimeVars(ctx context.Context) ([]string, error) {
	if d.localRunnerClient() == nil {
		return nil, nil
	}
	local, err := d.localRunnerClient().GlobalRuntimeVars(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get global runtime Vars on local")
	}
	remote, err := d.remoteRunnerClient().GlobalRuntimeVars(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get global runtime Vars on remote")
	}
	result := append(local, remote...)
	sort.Strings(result)
	return result, nil
}
