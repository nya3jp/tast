// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/logging"
)

// Service contains information about a gRPC service exported for remote tests.
type Service struct {
	// Register is a function called by the framework to register a gRPC service
	// to grpc.Server. This should be a simple function that constructs a gRPC
	// service implementation and calls pb.Register*Server.
	Register func(srv *grpc.Server, s *ServiceState)
	// Vars contains the names of runtime variables used by the service.
	Vars []string
}

// ServiceParams holds shared service params data for all gRPC services.
// The management data is managed by RPC Management Service, and used by all
// other user defined services.
type ServiceParams struct {
	// testVars has the run-time test variables passed in from the client.
	testVars map[string]string
}

// SetTestVars sets new testVars variable.
func (m *ServiceParams) SetTestVars(vars map[string]string) {
	m.testVars = vars
}

// Var returns the value for the named variable.
func (m *ServiceParams) Var(name string) (val string, ok bool) {
	val, ok = m.testVars[name]
	return val, ok
}

// ServiceState holds state relevant to a gRPC service.
type ServiceState struct {
	// ctx is a service-scoped context. It can be used to emit logs with
	// testing.ContextLog. It is canceled on gRPC server shutdown.
	ctx context.Context
	// svc is a pointer to the registered service instance.
	svc *Service
	// params points to the shared service params data.
	params *ServiceParams
}

// NewServiceState creates a new ServiceState.
func NewServiceState(ctx context.Context, svc *Service, params *ServiceParams) *ServiceState {
	return &ServiceState{
		ctx:    ctx,
		svc:    svc,
		params: params,
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

// Var returns the value for the named variable.
// The variable must be declared in the service definition with Vars.
// If a value was not supplied at runtime via the -var flag, false will be returned.
func (s *ServiceState) Var(name string) (val string, ok bool) {
	seen := false
	for _, n := range s.svc.Vars {
		if n == name {
			seen = true
			break
		}
	}
	if !seen {
		panic(fmt.Sprintf("Variable %q was not registered in service", name))
	}
	return s.params.Var(name)
}
