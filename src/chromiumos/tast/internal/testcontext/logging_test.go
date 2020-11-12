// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLogger(t *testing.T) {
	if _, ok := Logger(context.Background()); ok {
		t.Error("Logger(context.Background()) = true; want false")
	}

	var msgs []string
	sink := func(msg string) {
		msgs = append(msgs, msg)
	}

	ctx := WithLogger(context.Background(), sink)
	sink2, ok := Logger(ctx)
	if !ok {
		t.Fatal("Logger(ctx) = false; want true")
	}

	sink2("foo")
	sink2("bar")

	exp := []string{
		"foo",
		"bar",
	}
	if diff := cmp.Diff(msgs, exp); diff != "" {
		t.Error("Unexpected msgs (-got +want):\n", diff)
	}
}

func TestLog(t *testing.T) {
	// It is okay to call Log with a context not associated with a log sink.
	Log(context.Background(), "ab")
	Logf(context.Background(), "c%s", "d")

	var msgs []string
	sink := func(msg string) {
		msgs = append(msgs, msg)
	}
	ctx := WithLogger(context.Background(), sink)

	Log(ctx, "ef")
	Logf(ctx, "g%s", "h")

	exp := []string{
		"ef",
		"gh",
	}
	if diff := cmp.Diff(msgs, exp); diff != "" {
		t.Error("Unexpected msgs (-got +want):\n", diff)
	}
}
