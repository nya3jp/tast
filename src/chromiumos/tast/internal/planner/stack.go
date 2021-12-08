// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"

	"chromiumos/tast/internal/planner/internal/fixture"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

type internalOrCombinedStack struct {
	internal *fixture.InternalStack
	combined *fixture.CombinedStack
}

func (s *internalOrCombinedStack) Status() fixture.Status {
	if s.internal != nil {
		return s.internal.Status()
	}
	return s.combined.Status()
}

func (s *internalOrCombinedStack) Errors() []*protocol.Error {
	if s.internal != nil {
		return s.internal.Errors()
	}
	return s.combined.Errors()
}

func (s *internalOrCombinedStack) Val() interface{} {
	if s.internal != nil {
		return s.internal.Val()
	}
	return s.combined.Val()
}

func (s *internalOrCombinedStack) Push(ctx context.Context, fixt *testing.FixtureInstance) error {
	if s.internal != nil {
		return s.internal.Push(ctx, fixt)
	}
	return s.combined.Push(ctx, fixt)
}

func (s *internalOrCombinedStack) Pop(ctx context.Context) error {
	if s.internal != nil {
		return s.internal.Pop(ctx)
	}
	return s.combined.Pop(ctx)
}

func (s *internalOrCombinedStack) Reset(ctx context.Context) error {
	if s.internal != nil {
		return s.internal.Reset(ctx)
	}
	return s.combined.Reset(ctx)
}

func (s internalOrCombinedStack) PreTest(ctx context.Context, test *protocol.Entity, outDir string, out testing.OutputStream, condition *testing.EntityCondition) (func(ctx context.Context) error, error) {
	if s.internal != nil {
		return s.internal.PreTest(ctx, outDir, out, condition)
	}
	return s.combined.PreTest(ctx, test, outDir, out, condition)
}

func (s *internalOrCombinedStack) MarkDirty(ctx context.Context) error {
	if s.internal != nil {
		return s.internal.MarkDirty()
	}
	return s.combined.SetDirty(ctx, true)
}
