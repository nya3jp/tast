// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// ExternalStack operates fixtures in a remote bundle.
type ExternalStack struct {
	cl *client

	status Status            // updated on Reset
	errors []*protocol.Error // cached as a constant value
}

// NewExternalStack creates a new ExternalStack.
func NewExternalStack(ctx context.Context, srv protocol.TestService_RunTestsServer) (*ExternalStack, error) {
	cl := &client{srv: srv}

	status, err := cl.status(ctx)
	if err != nil {
		return nil, err
	}
	errors, err := cl.errors(ctx)
	if err != nil {
		return nil, err
	}

	return &ExternalStack{
		cl:     cl,
		status: status,
		errors: errors,
	}, nil
}

type client struct {
	srv protocol.TestService_RunTestsServer
}

func (op *client) do(ctx context.Context, req *protocol.StackOperationRequest) (*protocol.StackOperationResponse, error) {
	if err := op.srv.Send(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_StackOperation{
			StackOperation: req,
		},
	}); err != nil {
		return nil, err
	}
	resp, err := op.srv.Recv()
	if err != nil {
		return nil, err
	}
	if _, ok := resp.Type.(*protocol.RunTestsRequest_StackOperationResponse); !ok {
		return nil, fmt.Errorf("unexpected return type %T", resp.Type)
	}
	return resp.GetStackOperationResponse(), nil
}

func (op *client) status(ctx context.Context) (Status, error) {
	resp, err := op.do(ctx, &protocol.StackOperationRequest{
		Type: &protocol.StackOperationRequest_Status{
			Status: &protocol.StackGetStatus{},
		},
	})
	if err != nil {
		return 0, errors.Wrap(err, "stack status")
	}
	return statusFromProto(resp.GetStatus()), nil
}

func (op *client) errors(ctx context.Context) ([]*protocol.Error, error) {
	resp, err := op.do(ctx, &protocol.StackOperationRequest{
		Type: &protocol.StackOperationRequest_Errors{
			Errors: &protocol.StackGetErrors{},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "stack errors")
	}
	return resp.GetErrors(), nil
}

// Status returns status of the underlying stack.
func (s *ExternalStack) Status() Status {
	return s.status
}

// Errors returns errors of the underlying stack.
func (s *ExternalStack) Errors() []*protocol.Error {
	return s.errors
}

// Reset runs Reset on the underlying stack.
func (s *ExternalStack) Reset(ctx context.Context) error {
	resp, err := s.cl.do(ctx, &protocol.StackOperationRequest{
		Type: &protocol.StackOperationRequest_Reset_{
			Reset_: &protocol.StackReset{},
		},
	})
	if err != nil {
		return err
	}
	s.status = statusFromProto(resp.GetStatus())
	return nil
}

// PreTest runs PreTest on the underlying stack.
func (s *ExternalStack) PreTest(ctx context.Context, test *protocol.Entity, condition *testing.EntityCondition) (func(context.Context) error, error) {
	resp, err := s.cl.do(ctx, &protocol.StackOperationRequest{
		Type: &protocol.StackOperationRequest_PreTest{
			PreTest: &protocol.StackPreTest{
				Entity:   test,
				HasError: condition.HasError(),
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "stack PreTest")
	}
	if resp.GetTestHasError() {
		condition.RecordError()
	}
	return func(ctx context.Context) error {
		resp, err := s.cl.do(ctx, &protocol.StackOperationRequest{
			Type: &protocol.StackOperationRequest_PostTest{
				PostTest: &protocol.StackPostTest{
					Entity:   test,
					HasError: condition.HasError(),
				},
			},
		})
		if err != nil {
			return errors.Wrap(err, "stack PostTest")
		}
		if resp.GetTestHasError() {
			condition.RecordError()
		}
		return nil
	}, nil
}

// SetDirty runs SetDirty on the underlying stack.
func (s *ExternalStack) SetDirty(ctx context.Context, dirty bool) error {
	_, err := s.cl.do(ctx, &protocol.StackOperationRequest{
		Type: &protocol.StackOperationRequest_SetDirty{
			SetDirty: &protocol.StackSetDirty{
				Dirty: dirty,
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "stack set dirty")
	}
	return nil
}

// Val returns nil. TODO(oka): support remote fixture value.
func (s *ExternalStack) Val() (val interface{}) {
	return nil
}