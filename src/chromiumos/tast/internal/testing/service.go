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
	// GuaranteeCompatibility indicates that the service needs to strictly adhere to
	// the backward and forward compatibility guarantees when evolving proto definition
	// and implementation.
	// Once the flag is marked as True, it cannot be change to False in subsequent
	// versions.
	// The service will be exposed to non-Tast test harness clients.
	GuaranteeCompatibility bool
}

// ServiceRoot is the root of service state data.
type ServiceRoot struct {
	// svc is the registered service instance.
	service *Service
	// vars has the runtime variables.
	vars map[string]string
}

// NewServiceRoot creates a new ServiceRoot object.
func NewServiceRoot(svc *Service, vars map[string]string) *ServiceRoot {
	return &ServiceRoot{service: svc, vars: vars}
}

// ServiceState holds state relevant to a gRPC service.
type ServiceState struct {
	// ctx is a service-scoped context. It can be used to emit logs with
	// testing.ContextLog. It is canceled on gRPC server shutdown.
	ctx context.Context

	root *ServiceRoot
}

// NewServiceState creates a new ServiceState.
func NewServiceState(ctx context.Context, root *ServiceRoot) *ServiceState {
	return &ServiceState{
		ctx:  ctx,
		root: root,
	}
}

// Log formats its arguments using default formatting and logs them.
// Logs are sent to the currently connected remote bundle.
func (s *ServiceState) Log(args ...interface{}) {
	logging.Info(s.ctx, args...)
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
// Logs are sent to the currently connected remote bundle.
func (s *ServiceState) Logf(format string, args ...interface{}) {
	logging.Infof(s.ctx, format, args...)
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
	for _, n := range s.root.service.Vars {
		if n == name {
			seen = true
			break
		}
	}
	if !seen {
		panic(fmt.Sprintf("Variable %q was not registered in service", name))
	}
	val, ok = s.root.vars[name]
	return val, ok
}
