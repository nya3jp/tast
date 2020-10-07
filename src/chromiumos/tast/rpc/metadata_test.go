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
	ec := &testing.CurrentEntity{
		OutDir:          "/mock/outdir",
		HasSoftwareDeps: true,
		SoftwareDeps:    []string{"chrome", "android_p"},
		ServiceDeps:     []string{"tast.core.Ping"},
	}

	ctx := testing.WithCurrentEntity(context.Background(), ec)
	md := outgoingMetadata(ctx)

	exp := metadata.MD{
		metadataSoftwareDeps:    ec.SoftwareDeps,
		metadataHasSoftwareDeps: []string{"1"},
	}
	if diff := cmp.Diff(md, exp); diff != "" {
		t.Errorf("outgoingMetadata returned unexpected MD (-got +want):\n%s", diff)
	}
}

func TestOutgoingMetadataNoSoftwareDeps(t *gotesting.T) {
	ec := &testing.CurrentEntity{
		OutDir:      "/mock/outdir",
		ServiceDeps: []string{"tast.core.Ping"},
	}

	ctx := testing.WithCurrentEntity(context.Background(), ec)
	md := outgoingMetadata(ctx)

	exp := metadata.MD{
		metadataSoftwareDeps: nil,
	}
	if diff := cmp.Diff(md, exp); diff != "" {
		t.Errorf("outgoingMetadata returned unexpected MD (-got +want):\n%s", diff)
	}
}

func TestIncomingCurrentEntity(t *gotesting.T) {
	md := metadata.MD{
		metadataHasSoftwareDeps: []string{"1"},
		metadataSoftwareDeps:    []string{"chrome", "android_p"},
	}

	ec := incomingCurrentContext(md)

	exp := &testing.CurrentEntity{
		HasSoftwareDeps: true,
		SoftwareDeps:    md[metadataSoftwareDeps],
	}
	if diff := cmp.Diff(ec, exp); diff != "" {
		t.Errorf("incomingCurrentContext returned unexpected CurrentEntity (-got +want):\n%s", diff)
	}
}

func TestIncomingCurrentEntityNoSoftwareDeps(t *gotesting.T) {
	md := metadata.MD{
		metadataSoftwareDeps: nil,
	}

	ec := incomingCurrentContext(md)

	exp := &testing.CurrentEntity{}
	if diff := cmp.Diff(ec, exp); diff != "" {
		t.Errorf("incomingCurrentContext returned unexpected CurrentEntity (-got +want):\n%s", diff)
	}
}
