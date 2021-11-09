// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"

	"google.golang.org/grpc/metadata"

	"chromiumos/tast/internal/testcontext"
)

// Keys of metadata.MD. Allowed characters are [a-z0-9._-].
const (
	metadataSoftwareDeps    = "tast-testcontext-softwaredeps"
	metadataHasSoftwareDeps = "tast-testcontext-hassoftwaredeps"
	metadataLabels          = "tast-testcontext-labels"
	metadataTiming          = "tast-timing"
	metadataOutDir          = "tast-outdir"
	metadataLogLastSeq      = "tast-log-last-seq"
)

// outgoingMetadata extracts CurrentEntity from ctx and converts it to metadata.MD.
// It is called on gRPC clients to forward CurrentEntity over gRPC.
func outgoingMetadata(ctx context.Context) metadata.MD {
	swDeps, hasSwDeps := testcontext.SoftwareDeps(ctx)
	labels, _ := testcontext.Labels(ctx)
	md := metadata.MD{
		metadataSoftwareDeps: swDeps,
		metadataLabels:       labels,
	}
	if hasSwDeps {
		md[metadataHasSoftwareDeps] = []string{"1"}
	}
	return md
}

// incomingCurrentContext creates CurrentEntity from metadata.MD.
// It is called on gRPC servers to forward CurrentEntity over gRPC.
func incomingCurrentContext(md metadata.MD, outDir string) *testcontext.CurrentEntity {
	hasSoftwareDeps := len(md[metadataHasSoftwareDeps]) > 0
	softwareDeps := md[metadataSoftwareDeps]
	labels := md[metadataLabels]
	return &testcontext.CurrentEntity{
		OutDir:          outDir,
		HasSoftwareDeps: hasSoftwareDeps,
		SoftwareDeps:    softwareDeps,
		// ServiceDeps is not forwarded.
		Labels: labels,
	}
}
