// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/metadata"

	"chromiumos/tast/internal/testing"
)

func TestOutgoingMetadata(t *gotesting.T) {
	tc := &testing.TestContext{
		OutDir:       "/mock/outdir",
		SoftwareDeps: []string{"chrome", "android_p"},
		ServiceDeps:  []string{"tast.core.Ping"},
	}

	ctx := testing.WithTestContext(context.Background(), tc)
	md, err := outgoingMetadata(ctx)
	if err != nil {
		t.Fatal("outgoingMetadata failed: ", err)
	}

	exp := metadata.MD{
		metadataSoftwareDeps: tc.SoftwareDeps,
	}
	if diff := cmp.Diff(md, exp); diff != "" {
		t.Errorf("outgoingMetadata returned unexpected MD (-got +want):\n%s", diff)
	}
}

func TestOutgoingMetadataNoContext(t *gotesting.T) {
	_, err := outgoingMetadata(context.Background())
	if err == nil {
		t.Fatal("outgoingMetadata unexpectedly succeeded")
	}
}

func TestIncomingTestContext(t *gotesting.T) {
	md := metadata.MD{
		metadataSoftwareDeps: []string{"chrome", "android_p"},
	}

	tc := incomingTestContext(md)

	exp := &testing.TestContext{
		SoftwareDeps: md[metadataSoftwareDeps],
	}
	if diff := cmp.Diff(tc, exp); diff != "" {
		t.Errorf("incomingTestContext returned unexpected TestContext (-got +want):\n%s", diff)
	}
}
