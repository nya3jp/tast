// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/logging"
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
	// ctx is a service-scoped context. It can be used to emit logs with
	// testing.ContextLog. It is canceled on gRPC server shutdown.
	ctx context.Context
}

// NewServiceState creates a new ServiceState.
func NewServiceState(ctx context.Context) *ServiceState {
	return &ServiceState{
		ctx: ctx,
	}
}

// Log formats its arguments using default formatting and logs them.
// Logs are sent to the currently connected remote bundle.
func (s *ServiceState) Log(args ...interface{}) {
	logging.ContextLog(s.ctx, args...)
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
// Logs are sent to the currently connected remote bundle.
func (s *ServiceState) Logf(format string, args ...interface{}) {
	logging.ContextLogf(s.ctx, format, args...)
}

// ServiceContext returns a service-scoped context. A service-scoped context is
// canceled on gRPC service shutdown, while a context passed to a gRPC method is
// canceled on completion of the gRPC method call. Therefore a service-scoped
// context can be used to run background operations that span across multiple
// gRPC calls (e.g. starting a background subprocess).
// A service-scoped context can also be used with testing.ContextLog to send
// logs to the currently connected remote bundle.
func (s *ServiceState) ServiceContext() context.Context {
	return s.ctx
}
