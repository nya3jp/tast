// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"

	empty "github.com/golang/protobuf/ptypes/empty"

	"chromiumos/tast/internal/testing"
)

// managementServer implements the tast.core.Management gRPC service.
//
// It is provided by gRPC servers to let client communicates management info.
type managementServer struct {
	// md points to the service management data block.
	md *testing.ServiceManagementData
}

func newManagementServer(md *testing.ServiceManagementData) *managementServer {
	return &managementServer{
		md: md,
	}
}

func (s *managementServer) SetTestVars(ctx context.Context, testVars *TestVars) (*empty.Empty, error) {
	s.md.SetTestVars(testVars.GetVar())
	return &empty.Empty{}, nil
}

var _ ManagementServer = (*managementServer)(nil)
