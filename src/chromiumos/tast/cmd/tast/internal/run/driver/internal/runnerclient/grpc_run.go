// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"io"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// RunTests requests to run tests according to the given RunConfig.
// Test execution events are streamed back via out. See RunTestsOutput for
// details.
func (c *Client) RunTests(ctx context.Context, bcfg *protocol.BundleConfig, rcfg *protocol.RunConfig, out RunTestsOutput) {
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
			RunnerInitParams: c.params,
			BundleInitParams: &protocol.BundleInitParams{
				BundleConfig: bcfg,
				Vars:         rcfg.GetFeatures().GetInfra().GetVars(),
			},
		})
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
			DebugPort: rcfg.DebugPort,
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
			if err := handleEvent(ctx, res, out); err != nil {
				return err
			}
		}
	}())
}

func handleEvent(ctx context.Context, res *protocol.RunTestsResponse, out RunTestsOutput) error {
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
	case *protocol.RunTestsResponse_Heartbeat:
		return nil
	default:
		return errors.Errorf("unknown event type %T", res.GetType())
	}
}
