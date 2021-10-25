// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/planner/internal/entity"
	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// StackServerConfig contains static information the server should have to
// run stack methods.
type StackServerConfig struct {
	// Out is the output stream.
	Out output.Stream
	// Stack is the stack to operate.
	Stack *CombinedStack
	// OutDir is the common output directory.
	OutDir string

	CloudStorage *testing.CloudStorage
	RemoteData   *testing.RemoteData
}

// StackServer provides handling of stack operation requests from local bundles.
type StackServer struct {
	cfg      *StackServerConfig
	postTest func(context.Context, *protocol.StackPostTest) (*protocol.StackOperationResponse, error)
}

// NewStackServer creates a new StackServer.
func NewStackServer(cfg *StackServerConfig) *StackServer {
	return &StackServer{cfg: cfg}
}

// Handle handles a stack operation request.
// If a framework error happens during the handling, FatalError field in the
// response will be populated with the error message.
// If FatalError is empty, it means the operation has been successful.
func (s *StackServer) Handle(ctx context.Context, op *protocol.StackOperationRequest) *protocol.StackOperationResponse {
	var resp *protocol.StackOperationResponse
	var err error
	switch x := op.Type.(type) {
	case *protocol.StackOperationRequest_Reset_:
		resp, err = s.Reset(ctx)
	case *protocol.StackOperationRequest_PreTest:
		resp, err = s.PreTest(ctx, x.PreTest)
	case *protocol.StackOperationRequest_PostTest:
		resp, err = s.PostTest(ctx, x.PostTest)
	case *protocol.StackOperationRequest_Status:
		resp, err = s.Status(ctx)
	case *protocol.StackOperationRequest_SetDirty:
		resp, err = s.SetDirty(ctx, x.SetDirty)
	case *protocol.StackOperationRequest_Errors:
		resp, err = s.Errors(ctx)
	default:
		err = fmt.Errorf("BUG: unknown type %T", op)
	}
	if err != nil {
		return &protocol.StackOperationResponse{
			FatalError: err.Error(),
		}
	}
	return resp
}

// Reset runs Reset on stack.
func (s *StackServer) Reset(ctx context.Context) (*protocol.StackOperationResponse, error) {
	if err := s.cfg.Stack.Reset(ctx); err != nil {
		return nil, err
	}
	return &protocol.StackOperationResponse{
		Status: s.cfg.Stack.Status().proto(),
	}, nil
}

// PreTest constructs a TestEntityRoot, and calls PreTest on the stack.
// It stores a callback function for PostTest, so to invoke it on the next
// PostTest request. It's an error to call PreTest before PostTest is called
// after a successful PreTest.
func (s *StackServer) PreTest(ctx context.Context, req *protocol.StackPreTest) (*protocol.StackOperationResponse, error) {
	if s.postTest != nil {
		return nil, errors.New("BUG: PreTest called without PostTest called after successful PreTest")
	}
	test := req.GetEntity()
	if test == nil {
		return nil, errors.New("PreTest: no test set")
	}

	outDir, err := entity.CreateOutDir(s.cfg.OutDir, test.GetName())
	if err != nil {
		return nil, err
	}
	out := output.NewEntityStream(s.cfg.Out, test)

	testCtx, cancel := context.WithCancel(ctx)

	condition := testing.NewEntityCondition()
	if req.GetHasError() {
		condition.RecordError()
	}
	postTest, err := s.cfg.Stack.PreTest(testCtx, test, outDir, out, condition)
	if err != nil {
		cancel()
		return nil, err
	}
	s.postTest = func(ctx context.Context, req *protocol.StackPostTest) (*protocol.StackOperationResponse, error) {
		defer cancel()
		if req.GetHasError() {
			condition.RecordError()
		}
		if err := postTest(testCtx); err != nil {
			return nil, err
		}
		return &protocol.StackOperationResponse{
			TestHasError: condition.HasError(),
		}, nil
	}
	return &protocol.StackOperationResponse{
		TestHasError: condition.HasError(),
	}, nil
}

// PostTest runs PostTest on stack.
func (s *StackServer) PostTest(ctx context.Context, req *protocol.StackPostTest) (*protocol.StackOperationResponse, error) {
	if s.postTest == nil {
		return nil, fmt.Errorf("BUG: PostTest should be called after PreTest")
	}
	res, err := s.postTest(ctx, req)
	if err != nil {
		return nil, err
	}
	s.postTest = nil
	return res, nil
}

// Status runs Status on stack.
func (s *StackServer) Status(ctx context.Context) (*protocol.StackOperationResponse, error) {
	var res protocol.StackStatus
	switch s.cfg.Stack.Status() {
	case StatusGreen:
		res = protocol.StackStatus_GREEN
	case StatusRed:
		res = protocol.StackStatus_RED
	case StatusYellow:
		res = protocol.StackStatus_YELLOW
	default:
		return nil, fmt.Errorf("BUG: unknown status %v", s.cfg.Stack.Status())
	}
	return &protocol.StackOperationResponse{
		Status: res,
	}, nil
}

// SetDirty runs SetDirty on stack.
// TODO(oka): Consider removing SetDirty as it's only for debugging and adds
// round-trips between bundles.
func (s *StackServer) SetDirty(ctx context.Context, req *protocol.StackSetDirty) (*protocol.StackOperationResponse, error) {
	if err := s.cfg.Stack.SetDirty(ctx, req.GetDirty()); err != nil {
		return nil, err
	}
	return &protocol.StackOperationResponse{}, nil
}

// Errors runs Errors on stack.
func (s *StackServer) Errors(ctx context.Context) (*protocol.StackOperationResponse, error) {
	return &protocol.StackOperationResponse{
		Errors: s.cfg.Stack.Errors(),
	}, nil
}
