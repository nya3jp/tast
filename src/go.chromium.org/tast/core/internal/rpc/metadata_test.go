// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/tast/core/internal/testcontext"
)

func TestOutgoingMetadata(t *gotesting.T) {
	ec := &testcontext.CurrentEntity{
		OutDir:          "/mock/outdir",
		HasSoftwareDeps: true,
		SoftwareDeps:    []string{"chrome", "android_p"},
		ServiceDeps:     []string{"tast.core.Ping"},
		PrivateAttr:     []string{"private-attr"},
	}

	ctx := testcontext.WithCurrentEntity(context.Background(), ec)
	md := outgoingMetadata(ctx)

	exp := metadata.MD{
		metadataSoftwareDeps:    ec.SoftwareDeps,
		metadataHasSoftwareDeps: []string{"1"},
		metadataPrivateAttr:     []string{"private-attr"},
	}
	if diff := cmp.Diff(md, exp); diff != "" {
		t.Errorf("outgoingMetadata returned unexpected MD (-got +want):\n%s", diff)
	}
}

func TestOutgoingMetadataNoSoftwareDeps(t *gotesting.T) {
	ec := &testcontext.CurrentEntity{
		OutDir:      "/mock/outdir",
		ServiceDeps: []string{"tast.core.Ping"},
	}

	ctx := testcontext.WithCurrentEntity(context.Background(), ec)
	md := outgoingMetadata(ctx)

	exp := metadata.MD{
		metadataSoftwareDeps: nil,
		metadataPrivateAttr:  nil,
	}
	if diff := cmp.Diff(md, exp); diff != "" {
		t.Errorf("outgoingMetadata returned unexpected MD (-got +want):\n%s", diff)
	}
}

func TestIncomingCurrentEntity(t *gotesting.T) {
	const outDir = "/path/to/out"
	md := metadata.MD{
		metadataHasSoftwareDeps: []string{"1"},
		metadataSoftwareDeps:    []string{"chrome", "android_p"},
		metadataPrivateAttr:     []string{"private-attr"},
	}

	ec := incomingCurrentContext(md, outDir)

	exp := &testcontext.CurrentEntity{
		OutDir:          outDir,
		HasSoftwareDeps: true,
		SoftwareDeps:    md[metadataSoftwareDeps],
		PrivateAttr:     md[metadataPrivateAttr],
	}
	if diff := cmp.Diff(ec, exp); diff != "" {
		t.Errorf("incomingCurrentContext returned unexpected CurrentEntity (-got +want):\n%s", diff)
	}
}

func TestIncomingCurrentEntityNoSoftwareDeps(t *gotesting.T) {
	const outDir = "/path/to/out"
	md := metadata.MD{
		metadataSoftwareDeps: nil,
	}

	ec := incomingCurrentContext(md, outDir)

	exp := &testcontext.CurrentEntity{
		OutDir: outDir,
	}
	if diff := cmp.Diff(ec, exp); diff != "" {
		t.Errorf("incomingCurrentContext returned unexpected CurrentEntity (-got +want):\n%s", diff)
	}
}
