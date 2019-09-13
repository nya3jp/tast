// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"google.golang.org/grpc"
)

type Service struct {
	Register func(srv *grpc.Server, s *ServiceState)
}

// ServiceState holds state relevant to a gRPC service.
type ServiceState struct {
	// Nothing is provided for now.
}
