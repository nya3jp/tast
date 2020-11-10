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
	metadataTiming          = "tast-timing"
)

// outgoingMetadata extracts CurrentEntity from ctx and converts it to metadata.MD.
// It is called on gRPC clients to forward CurrentEntity over gRPC.
func outgoingMetadata(ctx context.Context) metadata.MD {
	swDeps, hasSwDeps := testcontext.SoftwareDeps(ctx)
	md := metadata.MD{
		metadataSoftwareDeps: swDeps,
	}
	if hasSwDeps {
		md[metadataHasSoftwareDeps] = []string{"1"}
	}
	return md
}

// incomingCurrentContext creates CurrentEntity from metadata.MD.
// It is called on gRPC servers to forward CurrentEntity over gRPC.
func incomingCurrentContext(md metadata.MD) *testcontext.CurrentEntity {
	hasSoftwareDeps := len(md[metadataHasSoftwareDeps]) > 0
	softwareDeps := md[metadataSoftwareDeps]
	return &testcontext.CurrentEntity{
		// TODO(crbug.com/969627): Support OutDir.
		HasSoftwareDeps: hasSoftwareDeps,
		SoftwareDeps:    softwareDeps,
		// ServiceDeps is not forwarded.
	}
}
