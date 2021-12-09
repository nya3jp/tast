// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundleclient

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// RunFixtureOutput is implemented by callers of RunFixture to receive fixture
// execution events.
type RunFixtureOutput interface {
	EntityLog(ctx context.Context, ev *protocol.EntityLogEvent) error
	EntityError(ctx context.Context, ev *protocol.EntityErrorEvent) error
}

// RunFixture requests a test bundle to set up a fixture.
// After successful return of RunFixture, a caller must call
// FixtureTicket.TearDown to tear down the fixture.
func (c *Client) RunFixture(ctx context.Context, name string, cfg *protocol.RunFixtureConfig, out RunFixtureOutput) (_ *FixtureTicket, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrapf(retErr, "failed to set up fixture %s", name)
		}
	}()

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{
		BundleInitParams: &protocol.BundleInitParams{
			Vars: cfg.GetTestVars(),
		},
	}, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			defer conn.Close(ctx)
		}
	}()

	stream, err := protocol.NewFixtureServiceClient(conn.Conn()).RunFixture(ctx)
	if err != nil {
		return nil, err
	}

	// Set up a remote fixture.
	req := &protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Push{
			Push: &protocol.RunFixturePushRequest{
				Name:   name,
				Config: cfg,
			},
		},
	}
	pushErrors, err := sendAndRecv(ctx, stream, name, req, out)
	if err != nil {
		return nil, err
	}

	startErrors := make([]*protocol.Error, len(pushErrors))
	for i, e := range pushErrors {
		startErrors[i] = &protocol.Error{
			Reason: e.GetReason(),
			Location: &protocol.ErrorLocation{
				File:  e.GetFile(),
				Line:  int64(e.GetLine()),
				Stack: e.GetStack(),
			},
		}
	}
	state := &protocol.StartFixtureState{
		Name:   name,
		Errors: startErrors,
	}
	return &FixtureTicket{
		out:    out,
		conn:   conn,
		stream: stream,
		state:  state,
	}, nil
}

// FixtureTicket tracks the state of a fixture set up by Client.RunFixture.
type FixtureTicket struct {
	out    RunFixtureOutput
	conn   *rpcConn
	stream protocol.FixtureService_RunFixtureClient
	state  *protocol.StartFixtureState
}

// TearDown tears down a fixture set up by Client.RunFixture and releases
// resources associated with the fixture.
// For a valid instance of FixtureTicket, TearDown must be called exactly once.
func (t *FixtureTicket) TearDown(ctx context.Context) (retErr error) {
	defer func() {
		if err := t.conn.Close(ctx); err != nil && retErr == nil {
			retErr = err
		}
		if retErr != nil {
			retErr = errors.Wrapf(retErr, "failed to tear down fixture %s", t.state.GetName())
		}
	}()

	req := &protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Pop{
			Pop: &protocol.RunFixturePopRequest{},
		},
	}
	if _, err := sendAndRecv(ctx, t.stream, t.state.GetName(), req, t.out); err != nil {
		return err
	}
	return nil
}

// StartFixtureState returns a StartFixtureState suitable to drive local test
// bundles.
func (t *FixtureTicket) StartFixtureState() *protocol.StartFixtureState {
	return t.state
}

func sendAndRecv(ctx context.Context, stream protocol.FixtureService_RunFixtureClient, name string, req *protocol.RunFixtureRequest, out RunFixtureOutput) ([]*protocol.RunFixtureError, error) {
	// Send a request.
	if err := stream.Send(req); err != nil {
		return nil, err
	}

	// Read responses until RequestDone.
	var errors []*protocol.RunFixtureError
	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil, err
		}

		switch v := msg.Control.(type) {
		case *protocol.RunFixtureResponse_Log:
			ev := &protocol.EntityLogEvent{
				Time:       msg.GetTimestamp(),
				EntityName: name,
				Text:       v.Log,
			}
			if err := out.EntityLog(ctx, ev); err != nil {
				return nil, err
			}
		case *protocol.RunFixtureResponse_Error:
			e := v.Error
			ev := &protocol.EntityErrorEvent{
				Time:       msg.GetTimestamp(),
				EntityName: name,
				Error: &protocol.Error{
					Reason: e.GetReason(),
					Location: &protocol.ErrorLocation{
						File:  e.GetFile(),
						Line:  int64(e.GetLine()),
						Stack: e.GetStack(),
					},
				},
			}
			if err := out.EntityError(ctx, ev); err != nil {
				return nil, err
			}
			errors = append(errors, e)
		case *protocol.RunFixtureResponse_RequestDone:
			return errors, nil
		}
	}
}
