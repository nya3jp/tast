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
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// EventCmpOpts is a list of options to be passed to cmp.Diff to compare
// protocol.Event slices ignoring non-deterministic fields.
var EventCmpOpts = []cmp.Option{
	cmpopts.IgnoreTypes(&timestamp.Timestamp{}),
	cmpopts.IgnoreFields(protocol.EntityStartEvent{}, "OutDir"),
	cmpopts.IgnoreFields(protocol.EntityEndEvent{}, "TimingLog"),
	cmpopts.IgnoreFields(protocol.Error{}, "Location"),
}

// RunTestsForEvents calls RunTests on cl with cfg and returns a slice of
// events. wantLogs specifies whether RunLogEvent and EntityLogEvent should be
// included in the result.
func RunTestsForEvents(cl protocol.TestServiceClient, cfg *protocol.RunConfig, wantLogs bool) ([]protocol.Event, error) {
	srv, err := cl.RunTests(context.Background())
	if err != nil {
		return nil, err
	}

	req := &protocol.RunTestsRequest{
		Type: &protocol.RunTestsRequest_RunTestsInit{
			RunTestsInit: &protocol.RunTestsInit{
				RunConfig: cfg,
			},
		},
	}
	if err := srv.Send(req); err != nil {
		return nil, errors.Wrap(err, "failed to send RunTestsInit")
	}

	var es []protocol.Event
	for {
		res, err := srv.Recv()
		if err == io.EOF {
			return es, nil
		}
		if err != nil {
			return es, err
		}

		e, ok := ExtractEvent(res)
		if !ok {
			continue
		}

		if !wantLogs {
			if _, ok := e.(*protocol.RunLogEvent); ok {
				continue
			}
			if _, ok := e.(*protocol.EntityLogEvent); ok {
				continue
			}
		}

		es = append(es, e)
	}
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
