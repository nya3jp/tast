// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundleclient

import (
	"context"
	"io"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// RunTestsOutput is implemented by callers of RunTests to receive test
// execution events.
//
// Its methods (except RunStart and RunEnd) are called on receiving a
// corresponding test execution event. In case of errors, they can be called in
// an inconsistent way (e.g. EntityEnd is not called after EntityStart due to a
// test crash). RunTestsOutput implementations must be prepared to handle such
// error cases correctly.
//
// All methods except RunEnd can return an error, which leads to immediate
// abort of the test execution and subsequent RunEnd call.
type RunTestsOutput interface {
	// RunStart is called exactly once at the beginning of an overall test
	// execution.
	RunStart(ctx context.Context) error

	// EntityStart is called when an entity starts.
	EntityStart(ctx context.Context, ev *protocol.EntityStartEvent) error
	// EntityLog is called with an entity log.
	EntityLog(ctx context.Context, ev *protocol.EntityLogEvent) error
	// EntityError is called with an entity error.
	EntityError(ctx context.Context, ev *protocol.EntityErrorEvent) error
	// EntityEnd is called when an entity finishes.
	EntityEnd(ctx context.Context, ev *protocol.EntityEndEvent) error
	// EntityCopyEnd is called when copy of output files completes after an entity
	// finishes.
	EntityCopyEnd(ctx context.Context, ev *protocol.EntityCopyEndEvent) error
	// RunLog is called with a log not associated with an entity.
	RunLog(ctx context.Context, ev *protocol.RunLogEvent) error
	// StackOperation is called to request remote fixture stack operation.
	// This is called when a local bundle needs remote fixture operation.
	StackOperation(ctx context.Context, req *protocol.StackOperationRequest) *protocol.StackOperationResponse

	// RunEnd is called exactly once at the end of an overall test execution.
	// If any other method returns a non-nil error, test execution is aborted
	// immediately and RunEnd is called with the error.
	RunEnd(ctx context.Context, err error)
}

// RunTests requests to run tests according to the given RunConfig.
// Test execution events are streamed back via out. See RunTestsOutput for
// details.
func (c *Client) RunTests(ctx context.Context, bcfg *protocol.BundleConfig, rcfg *protocol.RunConfig, out RunTestsOutput, recursive bool) {
	// Call RunEnd exactly once on returning from this method.
	out.RunEnd(ctx, func() (runErr error) {
		// Make sure all subprocesses and goroutines exit upon returning from
		// this function.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// This must come first since we have to call RunStart even if we
		// fail to start running tests.
		if err := out.RunStart(ctx); err != nil {
			return errors.Wrap(err, "starting test driver")
		}

		conn, err := c.dial(ctx, &protocol.HandshakeRequest{
			BundleInitParams: &protocol.BundleInitParams{
				BundleConfig: bcfg,
				Vars:         rcfg.GetFeatures().GetInfra().GetVars(),
			},
		}, int(rcfg.DebugPort))
		if err != nil {
			return errors.Wrap(err, "starting test bundle")
		}
		defer conn.Close(ctx)

		cl := protocol.NewTestServiceClient(conn.Conn())
		stream, err := cl.RunTests(ctx)
		if err != nil {
			return errors.Wrap(err, "starting test run")
		}
		defer stream.CloseSend()

		init := &protocol.RunTestsInit{
			RunConfig: rcfg,
			Recursive: recursive,
		}
		if err := stream.Send(&protocol.RunTestsRequest{Type: &protocol.RunTestsRequest_RunTestsInit{RunTestsInit: init}}); err != nil {
			return errors.Wrap(err, "initializing test run")
		}

		for {
			res, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return errors.Wrap(err, "connection to test bundle broken")
			}
			if err := handleEvent(ctx, res, out, stream); err != nil {
				return err
			}
		}
	}())
}

func handleEvent(ctx context.Context, res *protocol.RunTestsResponse, out RunTestsOutput, stream protocol.TestService_RunTestsClient) error {
	switch t := res.GetType().(type) {
	case *protocol.RunTestsResponse_RunLog:
		return out.RunLog(ctx, t.RunLog)
	case *protocol.RunTestsResponse_EntityStart:
		return out.EntityStart(ctx, t.EntityStart)
	case *protocol.RunTestsResponse_EntityLog:
		return out.EntityLog(ctx, t.EntityLog)
	case *protocol.RunTestsResponse_EntityError:
		return out.EntityError(ctx, t.EntityError)
	case *protocol.RunTestsResponse_EntityEnd:
		return out.EntityEnd(ctx, t.EntityEnd)
	case *protocol.RunTestsResponse_EntityCopyEnd:
		return out.EntityCopyEnd(ctx, t.EntityCopyEnd)
	case *protocol.RunTestsResponse_StackOperation:
		resp := out.StackOperation(ctx, t.StackOperation)
		return stream.Send(&protocol.RunTestsRequest{
			Type: &protocol.RunTestsRequest_StackOperationResponse{
				StackOperationResponse: resp,
			},
		})
	case *protocol.RunTestsResponse_Heartbeat:
		return nil
	default:
		return errors.Errorf("unknown event type %T", res.GetType())
	}
}
