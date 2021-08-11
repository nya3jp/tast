// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package timing_test

import (
	"context"
	"testing"

	"chromiumos/tast/internal/timing"
)

func TestContext(t *testing.T) {
	if cl, cs, ok := timing.FromContext(context.Background()); ok || cl != nil || cs != nil {
		t.Errorf("FromContext(%v) = (%v, %v, %v); want (%v, %v, %v)", context.Background(), cl, cs, ok, nil, nil, false)
	}

	l := timing.NewLog()
	ctx := timing.NewContext(context.Background(), l)
	if cl, cs, ok := timing.FromContext(ctx); !ok || cl != l || cs != l.Root {
		t.Errorf("FromContext(%v) = (%v, %v, %v); want (%v, %v, %v)", ctx, cl, cs, ok, l, &l.Root, true)
	}
}

func TestStartNil(t *testing.T) {
	// Start should be okay with receiving a context without a Log attached to it,
	// and Stage.End should be okay with a nil receiver.
	_, st := timing.Start(context.Background(), "mystage")
	st.End()
}

func TestStartSeq(t *testing.T) {
	l := timing.NewLog()
	ctx := timing.NewContext(context.Background(), l)
	ctx1, st1 := timing.Start(ctx, "stage1")
	_, st2 := timing.Start(ctx1, "stage2")
	st2.End()
	st1.End()

	if len(l.Root.Children) != 1 {
		t.Errorf("Got %d stages; want 1", len(l.Root.Children))
	} else if l.Root.Children[0].Name != "stage1" {
		t.Errorf("Got stage %q; want %q", l.Root.Children[0].Name, "stage1")
	}

	if len(l.Root.Children[0].Children) != 1 {
		t.Errorf("Got %d stages; want 1", len(l.Root.Children[0].Children))
	} else if l.Root.Children[0].Children[0].Name != "stage2" {
		t.Errorf("Got stage %q; want %q", l.Root.Children[0].Children[0].Name, "stage2")
	}
}

func TestStartPar(t *testing.T) {
	l := timing.NewLog()
	ctx := timing.NewContext(context.Background(), l)
	_, st1 := timing.Start(ctx, "stage1")
	_, st2 := timing.Start(ctx, "stage2")
	st2.End()
	st1.End()

	if len(l.Root.Children) != 2 {
		t.Errorf("Got %d stages; want 2", len(l.Root.Children))
	} else {
		if l.Root.Children[0].Name != "stage1" {
			t.Errorf("Got stage %q; want %q", l.Root.Children[0].Name, "stage1")
		}
		if l.Root.Children[1].Name != "stage2" {
			t.Errorf("Got stage %q; want %q", l.Root.Children[1].Name, "stage2")
		}
	}
}
