// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/metadata"

	"chromiumos/tast/testing"
)

// Keys of metadata.MD. Allowed characters are [a-z0-9._-].
const (
	metadataSoftwareDeps = "tast-testcontext-softwaredeps"
)

// outgoingMetadata extracts TestContext from ctx and converts it to metadata.MD.
// It is called on gRPC clients to forward TestContext over gRPC.
func outgoingMetadata(ctx context.Context) (metadata.MD, error) {
	swDeps, ok := testing.ContextSoftwareDeps(ctx)
	if !ok {
		return nil, errors.New("SoftwareDeps not available (using a wrong context?)")
	}
	return metadata.MD{
		metadataSoftwareDeps: swDeps,
	}, nil
}

// incomingTestContext creates TestContext from metadata.MD.
// It is called on gRPC servers to forward TestContext over gRPC.
func incomingTestContext(md metadata.MD, logger func(msg string)) *testing.TestContext {
	softwareDeps := md[metadataSoftwareDeps]
	return &testing.TestContext{
		Logger: logger,
		// TODO(crbug.com/969627): Support OutDir.
		TestInfo: &testing.TestContextTestInfo{
			SoftwareDeps: softwareDeps,
			// ServiceDeps is not forwarded.
		},
	}
}
