// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"fmt"
	"io"

	"github.com/golang/protobuf/ptypes"
	empty "github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/dut"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// FixtureService implements FixtureService gRPC service.
type FixtureService struct {
}

// RunFixtureServiceServer runs a gRPC server providing fixture service on r/w channels.
// It blocks until the client connection is closed or it encounters an error.
func RunFixtureServiceServer(r io.Reader, w io.Writer) error {
	srv := grpc.NewServer()
	reflection.Register(srv)

	RegisterFixtureServiceServer(srv, &FixtureService{})

	if err := srv.Serve(rpc.NewPipeListener(r, w)); err != nil && err != io.EOF {
		return err
	}
	return nil
}

var _ FixtureServiceServer = (*FixtureService)(nil)

// fixtureServiceLogger implements planner.OutputStream.
type fixtureServiceLogger struct {
	stream FixtureService_RunFixtureServer
}

func (l *fixtureServiceLogger) Log(msg string) error {
	return l.stream.Send(&RunFixtureResponse{
		Control:   &RunFixtureResponse_Log{Log: msg},
		Timestamp: ptypes.TimestampNow(),
	})
}

func (l *fixtureServiceLogger) Error(e *testing.Error) error {
	return l.stream.Send(&RunFixtureResponse{
		Control: &RunFixtureResponse_Error{
			Error: &RunFixtureError{
				Reason: e.Reason,
				File:   e.File,
				Line:   int32(e.Line),
				Stack:  e.Stack,
			},
		},
		Timestamp: ptypes.TimestampNow(),
	})
}

func (l *fixtureServiceLogger) EntityStart(ei *testing.EntityInfo, outDir string) error {
	return nil
}

func (l *fixtureServiceLogger) EntityLog(ei *testing.EntityInfo, msg string) error {
	return l.Log(msg)
}

func (l *fixtureServiceLogger) EntityError(ei *testing.EntityInfo, e *testing.Error) error {
	return l.Error(e)
}

func (l *fixtureServiceLogger) EntityEnd(ei *testing.EntityInfo, skipReasons []string, timingLog *timing.Log) error {
	return nil
}

// RunFixture provides operations to set up and tear down fixtures.
func (s *FixtureService) RunFixture(srv FixtureService_RunFixtureServer) error {
	ctx := srv.Context()

	var stack *planner.FixtureStack

	for {
		req, err := srv.Recv()
		if err != nil {
			return err
		}
		switch v := req.Control.(type) {
		case *RunFixtureRequest_Push:
			if stack != nil {
				return fmt.Errorf("Push called twice in a row")
			}
			r := v.Push
			f := testing.GlobalRegistry().AllFixtures()[r.Name]
			if f == nil {
				return fmt.Errorf("fixture %v not found in %#v", r.Name, testing.GlobalRegistry().AllFixtures())
			}

			// Create DUT connection.
			dt, err := dut.New(r.Config.Target, r.Config.KeyFile, r.Config.KeyDir)
			if err != nil {
				return fmt.Errorf("failed to create DUT: %v", err)
			}
			defer dt.Close(ctx)

			pcfg := &planner.Config{
				DataDir: r.Config.DataDir,
				OutDir:  r.Config.OutDir,
				Vars:    r.Config.TestVars,
				RemoteData: &testing.RemoteData{
					RPCHint: testing.NewRPCHint(r.Config.LocalBundleDir, nil),
					DUT:     dt,
				},
			}
			if r.Config.CustomGracePeriod != nil {
				d, err := ptypes.Duration(r.Config.CustomGracePeriod)
				if err != nil {
					return fmt.Errorf("invalid CustomGracePeriod: %v", err)
				}
				pcfg.CustomGracePeriod = d
			}

			stack = planner.NewFixtureStack(pcfg, &fixtureServiceLogger{srv})
			if err := stack.Push(ctx, f); err != nil {
				return err
			}

			if err := srv.Send(&RunFixtureResponse{
				Control: &RunFixtureResponse_RequestDone{
					RequestDone: &empty.Empty{},
				},
				Timestamp: ptypes.TimestampNow(),
			}); err != nil {
				return err
			}
		case *RunFixtureRequest_Pop:
			if stack == nil {
				return fmt.Errorf("Pop called before Push")
			}
			if err := stack.Pop(ctx); err != nil {
				return err
			}
			if err := srv.Send(&RunFixtureResponse{
				Control: &RunFixtureResponse_RequestDone{
					RequestDone: &empty.Empty{},
				},
				Timestamp: ptypes.TimestampNow(),
			}); err != nil {
				return err
			}
			stack = nil
		}
	}
}
