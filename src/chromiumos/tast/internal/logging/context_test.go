// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSinkFromContext(t *testing.T) {
	if _, ok := SinkFromContext(context.Background()); ok {
		t.Error("SinkFromContext(context.Background()) = true; want false")
	}

	var msgs []string
	sink := func(msg string) {
		msgs = append(msgs, msg)
	}

	ctx := NewContext(context.Background(), sink)
	sink2, ok := SinkFromContext(ctx)
	if !ok {
		t.Fatal("SinkFromContext(ctx) = false; want true")
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

func TestContextLog(t *testing.T) {
	// It is okay to call ContextLog with a context not associated with a log sink.
	ContextLog(context.Background(), "ab")
	ContextLogf(context.Background(), "c%s", "d")

	var msgs []string
	sink := func(msg string) {
		msgs = append(msgs, msg)
	}
	ctx := NewContext(context.Background(), sink)

	ContextLog(ctx, "ef")
	ContextLogf(ctx, "g%s", "h")

	exp := []string{
		"ef",
		"gh",
	}
	if diff := cmp.Diff(msgs, exp); diff != "" {
		t.Error("Unexpected msgs (-got +want):\n", diff)
	}
}
