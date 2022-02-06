// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package protocoltest provides utilities for unit tests involving Tast gRPC
// protocol.
package protocoltest

import (
	"context"
	"io"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// EventCmpOpts is a list of options to be passed to cmp.Diff to compare
// protocol.Event slices ignoring non-deterministic fields.
var EventCmpOpts = []cmp.Option{
	protocmp.Transform(),
	protocmp.IgnoreMessages(&timestamp.Timestamp{}),
	protocmp.IgnoreFields(&protocol.EntityStartEvent{}, "out_dir"),
	protocmp.IgnoreFields(&protocol.EntityEndEvent{}, "timing_log"),
	protocmp.IgnoreFields(&protocol.Error{}, "location"),
}

type config struct {
	wantRunLogs    bool
	wantEntityLogs bool
}

// Option represents an option to RunTestsForEvents.
type Option func(*config)

// WithRunLogs instructs to include RunLogEvents to the result.
func WithRunLogs() Option {
	return func(cfg *config) { cfg.wantRunLogs = true }
}

// WithEntityLogs instructs to include EntityLogEvents to the result.
func WithEntityLogs() Option {
	return func(cfg *config) { cfg.wantEntityLogs = true }
}

// RunTestsRecursiveForEvents is similar to RunTestsForEvents, but calls
// RunTests with a recursive flag.
func RunTestsRecursiveForEvents(ctx context.Context, cl protocol.TestServiceClient, rcfg *protocol.RunConfig, opts ...Option) ([]protocol.Event, error) {
	return runTestsForEvents(ctx, cl, rcfg, true, opts...)
}

// RunTestsForEvents calls RunTests on cl with cfg and returns a slice of
// events. wantLogs specifies whether RunLogEvent and EntityLogEvent should be
// included in the result.
func RunTestsForEvents(ctx context.Context, cl protocol.TestServiceClient, rcfg *protocol.RunConfig, opts ...Option) ([]protocol.Event, error) {
	return runTestsForEvents(ctx, cl, rcfg, false, opts...)
}

func runTestsForEvents(ctx context.Context, cl protocol.TestServiceClient, rcfg *protocol.RunConfig, recursive bool, opts ...Option) ([]protocol.Event, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	srv, err := cl.RunTests(ctx)
	if err != nil {
		return nil, err
	}

	req := &protocol.RunTestsRequest{
		Type: &protocol.RunTestsRequest_RunTestsInit{
			RunTestsInit: &protocol.RunTestsInit{
				RunConfig: rcfg,
				Recursive: recursive,
			},
		},
	}
	if err := srv.Send(req); err != nil {
		return nil, errors.Wrap(err, "failed to send RunTestsInit")
	}
	defer srv.CloseSend()

	var es []protocol.Event
	for {
		res, err := srv.Recv()
		if err == io.EOF {
			return es, nil
		}
		if err != nil {
			return es, err
		}

		if err := replyStackOperation(srv, res); err != nil {
			return nil, err
		}
		e, ok := ExtractEvent(res)
		if !ok {
			continue
		}

		if _, ok := e.(*protocol.RunLogEvent); ok && !cfg.wantRunLogs {
			continue
		}
		if _, ok := e.(*protocol.EntityLogEvent); ok && !cfg.wantEntityLogs {
			continue
		}

		es = append(es, e)
	}
}

func replyStackOperation(srv protocol.TestService_RunTestsClient, res *protocol.RunTestsResponse) error {
	if _, ok := res.Type.(*protocol.RunTestsResponse_StackOperation); !ok {
		return nil
	}
	var resp *protocol.StackOperationResponse
	switch res.GetStackOperation().Type.(type) {
	case *protocol.StackOperationRequest_Reset_:
		resp = &protocol.StackOperationResponse{Status: protocol.StackStatus_GREEN}
	case *protocol.StackOperationRequest_PreTest:
		resp = &protocol.StackOperationResponse{}
	case *protocol.StackOperationRequest_PostTest:
		resp = &protocol.StackOperationResponse{}
	case *protocol.StackOperationRequest_Status:
		resp = &protocol.StackOperationResponse{Status: protocol.StackStatus_GREEN}
	case *protocol.StackOperationRequest_SetDirty:
		resp = &protocol.StackOperationResponse{}
	case *protocol.StackOperationRequest_Errors:
		resp = &protocol.StackOperationResponse{}
	}
	return srv.Send(&protocol.RunTestsRequest{Type: &protocol.RunTestsRequest_StackOperationResponse{StackOperationResponse: resp}})
}

// ExtractEvent extracts Event from RunTestsResponse. It is useful in unit tests
// to compare received events against expectation.
func ExtractEvent(res *protocol.RunTestsResponse) (protocol.Event, bool) {
	switch res := res.GetType().(type) {
	case *protocol.RunTestsResponse_RunLog:
		return res.RunLog, true
	case *protocol.RunTestsResponse_EntityStart:
		return res.EntityStart, true
	case *protocol.RunTestsResponse_EntityLog:
		return res.EntityLog, true
	case *protocol.RunTestsResponse_EntityError:
		return res.EntityError, true
	case *protocol.RunTestsResponse_EntityEnd:
		return res.EntityEnd, true
	default:
		return nil, false
	}
}
