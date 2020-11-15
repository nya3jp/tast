// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// fixtureService implements FixtureServiceServer gRPC service.
type fixtureService struct {
}

var _ FixtureServiceServer = (*fixtureService)(nil)

// RunFixture provides operations to set up and tear down fixtures.
// It accepts multiple pairs of push and pop requests in a loop until client
// closes the connection.
func (s *fixtureService) RunFixture(srv FixtureService_RunFixtureServer) error {
	sendDone := func() error {
		return srv.Send(&RunFixtureResponse{
			Control: &RunFixtureResponse_RequestDone{
				RequestDone: &empty.Empty{},
			},
			Timestamp: ptypes.TimestampNow(),
		})
	}

	for {
		pop, err := s.push(srv)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("push: %v", err)
		}
		if err := sendDone(); err != nil {
			return err
		}

		if err := pop(); err != nil {
			return fmt.Errorf("pop: %v", err)
		}
		if err := sendDone(); err != nil {
			return err
		}
	}
}

func (*fixtureService) push(srv FixtureService_RunFixtureServer) (pop func() error, retErr error) {
	ctx := srv.Context()

	req, err := srv.Recv()
	if err != nil {
		return nil, err
	}
	r := req.GetPush()
	if r == nil {
		return nil, fmt.Errorf("req = %v, want push request", req)
	}

	f := testing.GlobalRegistry().AllFixtures()[r.Name]
	if f == nil {
		return nil, fmt.Errorf("fixture %v not found", r.Name)
	}

	removeTemp := false
	if r.Config.TempDir == "" {
		r.Config.TempDir, err = defaultTempDir()
		if err != nil {
			return nil, err
		}

		removeTemp = true
		defer func() {
			if retErr != nil {
				os.RemoveAll(r.Config.TempDir)
			}
		}()
	}

	restoreTempDir, err := prepareTempDir(r.Config.TempDir)
	if err != nil {
		return nil, err
	}
	defer restoreTempDir()

	testcontext.Log(ctx, "Connecting to DUT")
	dt, err := connectToTarget(ctx, r.Config.Target, r.Config.KeyFile, r.Config.KeyDir, nil)
	if err != nil {
		return nil, err
	}
	disconnect := func() {
		testcontext.Log(ctx, "Disconnecting from DUT")
		// It is OK to ignore the error since we've finished running fixture at this point.
		dt.Close(ctx)
	}
	defer func() {
		if retErr != nil {
			disconnect()
		}
	}()

	var downloadMode planner.DownloadMode
	switch r.Config.DownloadMode {
	case RunFixtureConfig_BATCH:
		downloadMode = planner.DownloadBatch
	case RunFixtureConfig_LAZY:
		downloadMode = planner.DownloadLazy
	}

	pcfg := &planner.Config{
		DataDir:           r.Config.DataDir,
		OutDir:            r.Config.OutDir,
		Devservers:        r.Config.Devservers,
		TLWServer:         r.Config.TlwServer,
		DUTName:           r.Config.DutName,
		BuildArtifactsURL: r.Config.BuildArtifactsUrl,
		RemoteData: &testing.RemoteData{
			RPCHint: testing.NewRPCHint(r.Config.LocalBundleDir, r.Config.TestVars),
			DUT:     dt,
			// TODO(oka): fill Meta field.
		},
		DownloadMode: downloadMode,
		Features: dep.Features{
			Var: r.Config.TestVars,
			Software: &dep.SoftwareFeatures{
				Available:   r.Config.AvailableSoftwareFeatures,
				Unavailable: r.Config.UnavailableSoftwareFeatures,
			},
			// TODO(oka): fill HardwareFeatures field.
		},
	}
	if r.Config.CustomGracePeriod != nil {
		d, err := ptypes.Duration(r.Config.CustomGracePeriod)
		if err != nil {
			return nil, fmt.Errorf("invalid CustomGracePeriod: %v", err)
		}
		pcfg.CustomGracePeriod = &d
	}

	stack := planner.NewFixtureStack(pcfg, &fixtureServiceLogger{srv})
	if err := stack.Push(ctx, f); err != nil {
		return nil, err
	}

	return func() (retErr error) {
		if removeTemp {
			defer func() {
				if err := os.RemoveAll(r.Config.TempDir); err != nil && retErr == nil {
					retErr = err
				}
			}()
		}

		defer disconnect()
		if !dt.Connected(ctx) {
			testcontext.Log(ctx, "Reconnecting to DUT")
			if err := dt.Connect(ctx); err != nil {
				return err
			}
		}

		req, err := srv.Recv()
		if err != nil {
			return err
		}
		if req.GetPop() == nil {
			return fmt.Errorf("req = %v, want pop request", req)
		}

		restoreTempDir, err := prepareTempDir(r.Config.TempDir)
		if err != nil {
			return err
		}
		defer restoreTempDir()

		if err := stack.Pop(ctx); err != nil {
			return err
		}
		return nil
	}, nil
}

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

// RunFixtureServiceServer runs a gRPC server providing fixture service on r/w channels.
// It blocks until the client connection is closed or it encounters an error.
func RunFixtureServiceServer(r io.Reader, w io.Writer) error {
	srv := grpc.NewServer()
	reflection.Register(srv)

	RegisterFixtureServiceServer(srv, &fixtureService{})

	if err := srv.Serve(rpc.NewPipeListener(r, w)); err != nil && err != io.EOF {
		return err
	}
	return nil
}
