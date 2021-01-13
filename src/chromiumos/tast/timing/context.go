// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package timing

import (
	"context"
)

type key int // unexported context.Context key type to avoid collisions with other packages

const (
	logKey          key = iota // key used for attaching a Log to a context.Context
	currentStageKey            // key used for attaching a current Stage to a context.Context
)

// NewContext returns a new context that carries l and its root stage as
// the current stage.
func NewContext(ctx context.Context, l *Log) context.Context {
	ctx = context.WithValue(ctx, logKey, l)
	ctx = context.WithValue(ctx, currentStageKey, l.Root)
	return ctx
}

// FromContext returns the Log and the current Stage stored in ctx, if any.
func FromContext(ctx context.Context) (*Log, *Stage, bool) {
	l, ok := ctx.Value(logKey).(*Log)
	if !ok {
		return nil, nil, false
	}
	s, ok := ctx.Value(currentStageKey).(*Stage)
	if !ok {
		return nil, nil, false
	}
	return l, s, true
}

// Start starts and returns a new Stage named name within the Log attached
// to ctx. If no Log is attached to ctx, nil is returned. It is safe to call Close
// on a nil stage.
//
// Example usage to report the time used until the end of the current function:
//
//	ctx, st := timing.Start(ctx, "my_stage")
//	defer st.End()
func Start(ctx context.Context, name string) (context.Context, *Stage) {
	_, s, ok := FromContext(ctx)
	if !ok {
		return ctx, nil
	}
	c := s.StartChild(name)
	return context.WithValue(ctx, currentStageKey, c), c
}
