// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package timing provides functions to record timing information.
package timing

import (
	"context"

	"go.chromium.org/tast/core/tastuseonly/timing"
)

// Stage represents a discrete unit of work that is being timed.
type Stage struct {
	st *timing.Stage
}

// End ends the stage. Child stages are recursively examined and also ended
// (although we expect them to have already been ended).
func (st *Stage) End() {
	st.st.End()
}

// Start starts and returns a new Stage named name.
//
// Example usage to report the time used until the end of the current function:
//
//	ctx, st := timing.Start(ctx, "my_stage")
//	defer st.End()
func Start(ctx context.Context, name string) (context.Context, *Stage) {
	ctx, st := timing.Start(ctx, name)
	return ctx, &Stage{st}
}
