// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"google.golang.org/grpc"
)

// Service contains information about a gRPC service exported for remote tests.
type Service struct {
	// Register is a function called by the framework to register a gRPC service
	// to grpc.Server. This should be a simple function that constructs a gRPC
	// service implementation and calls pb.Register*Server.
	Register func(srv *grpc.Server, s *ServiceState)
}

// ServiceState holds state relevant to a gRPC service.
type ServiceState struct {
	// TestVars is the test variables for the service
	TestVars map[string]string
}

// Var returns the value for the named variable.
// If a value was not supplied at runtime via the -var flag, ok will be false.
func (s *ServiceState) Var(name string) (val string, ok bool) {
	val, ok = s.TestVars[name]
	return val, ok
}
