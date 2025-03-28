// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/planner"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/internal/timing"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

// fixtureService implements FixtureServiceServer gRPC service.
type fixtureService struct {
	reg *testing.Registry
}

var _ protocol.FixtureServiceServer = (*fixtureService)(nil)

// registerFixtureService registers fixture service.
func registerFixtureService(srv *grpc.Server, reg *testing.Registry) {
	protocol.RegisterFixtureServiceServer(srv, &fixtureService{reg: reg})
}

// RunFixture provides operations to set up and tear down fixtures.
// It accepts multiple pairs of push and pop requests in a loop until client
// closes the connection.
func (s *fixtureService) RunFixture(srv protocol.FixtureService_RunFixtureServer) error {
	for {
		if err := s.pushAndPop(srv); err == errFixtureServiceNormalEOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

var errFixtureServiceNormalEOF = errors.New("normal EOF")

// pushAndPop handles push and pop operations. If the connection is terminated
// normally, it returns errFixtureServiceNormalEOF.
func (s *fixtureService) pushAndPop(srv protocol.FixtureService_RunFixtureServer) (retErr error) {
	ctx := srv.Context()

	sendDone := func() error {
		return srv.Send(&protocol.RunFixtureResponse{
			Control: &protocol.RunFixtureResponse_RequestDone{
				RequestDone: &emptypb.Empty{},
			},
			Timestamp: timestamppb.Now(),
		})
	}

	// sendDone for the pop operation is run after all other deferred
	// operations are done. This resolves timing issues in the unit test
	// TestFixtureServiceDefaultTempDir.
	defer func() {
		if retErr != nil {
			return
		}
		retErr = sendDone()
	}()

	req, err := srv.Recv()
	if err == io.EOF {
		return errFixtureServiceNormalEOF
	} else if err != nil {
		return err
	}
	r := req.GetPush()
	if r == nil {
		return fmt.Errorf("req = %v, want push request", req)
	}

	f := s.reg.AllFixtures()[r.Name]
	if f == nil {
		return fmt.Errorf("push %v: no such fixture", r.Name)
	}

	if r.Config.TempDir == "" {
		r.Config.TempDir, err = defaultTempDir()
		if err != nil {
			return fmt.Errorf("push %v: %v", r.Name, err)
		}
		defer func() {
			if err := os.RemoveAll(r.Config.TempDir); err != nil && retErr == nil {
				retErr = fmt.Errorf("push %v: %v", r.Name, err)
			}
		}()
	}

	restoreTempDir, err := prepareTempDir(r.Config.TempDir)
	if err != nil {
		return fmt.Errorf("push %v: %v", r.Name, err)
	}
	defer restoreTempDir()

	logging.Info(ctx, "Connecting to DUT")
	dt, err := connectToTarget(ctx, r.Config.ConnectionSpec, r.Config.KeyFile, r.Config.KeyDir, r.Config.ProxyCommand, nil)
	if err != nil {
		return fmt.Errorf("push %v: %v", r.Name, err)
	}
	defer func() {
		logging.Info(ctx, "Disconnecting from DUT")
		// It is OK to ignore the error since we've finished running fixture at this point.
		dt.Close(ctx)
	}()

	pcfg := &planner.Config{
		Dirs: &protocol.RunDirectories{
			DataDir: r.Config.DataDir,
			OutDir:  r.Config.OutDir,
		},
		Service: &protocol.ServiceConfig{
			Devservers:  r.Config.Devservers,
			DutServer:   r.Config.DutServer,
			TlwServer:   r.Config.TlwServer,
			TlwSelfName: r.Config.DutName,
		},
		DataFile: &protocol.DataFileConfig{
			BuildArtifactsUrl: r.Config.BuildArtifactsUrl,
			DownloadMode:      r.Config.GetDownloadMode(),
		},
		RemoteData: &testing.RemoteData{
			RPCHint: testing.NewRPCHint(r.Config.LocalBundleDir, r.Config.TestVars),
			DUT:     dt,
			KeyFile: r.Config.KeyFile,
			KeyDir:  r.Config.KeyDir,
			// TODO(oka): fill Meta field.
		},
		Features: &protocol.Features{
			Infra: &protocol.InfraFeatures{
				Vars:         r.Config.GetTestVars(),
				DUTLabConfig: r.Config.GetDUTLabConfig(),
			},
			Dut: &frameworkprotocol.DUTFeatures{
				Software: &frameworkprotocol.SoftwareFeatures{
					Available:   r.Config.GetAvailableSoftwareFeatures(),
					Unavailable: r.Config.GetUnavailableSoftwareFeatures(),
				},
				// TODO(oka): fill HardwareFeatures field.
			},
		},
	}
	if r.Config.CustomGracePeriod != nil {
		if err := r.Config.CustomGracePeriod.CheckValid(); err != nil {
			return fmt.Errorf("push %v: invalid CustomGracePeriod: %v", r.Name, err)
		}
		d := r.Config.CustomGracePeriod.AsDuration()
		pcfg.CustomGracePeriod = &d
	}

	stack := planner.NewFixtureStack(pcfg, &fixtureServiceLogger{srv})
	if dt != nil && !dt.Connected(ctx) {
		// Try to reconnect to the DUT.
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := dt.WaitConnect(waitConnectCtx); err != nil {
			return fmt.Errorf("failed to connect to DUT before running fixture %s: %v", r.Name, err)
		}
	}
	if err := stack.Push(ctx, f); err != nil {
		return fmt.Errorf("push %v: %v", r.Name, err)
	}
	if err := sendDone(); err != nil {
		return fmt.Errorf("push %v: %v", r.Name, err)
	}

	req, err = srv.Recv()
	if err != nil {
		return fmt.Errorf("pop %v: %v", r.Name, err)
	}
	if req.GetPop() == nil {
		return fmt.Errorf("req = %v, want pop for %v", req, r.Name)
	}

	if !dt.Connected(ctx) {
		logging.Info(ctx, "Reconnecting to DUT")
		if err := dt.Connect(ctx); err != nil {
			return fmt.Errorf("pop %v: %v", r.Name, err)
		}
	}

	if err := stack.Pop(ctx); err != nil {
		return fmt.Errorf("pop %v: %v", r.Name, err)
	}
	return nil
}

// fixtureServiceLogger implements planner.OutputStream.
type fixtureServiceLogger struct {
	stream protocol.FixtureService_RunFixtureServer
}

func (l *fixtureServiceLogger) Log(level logging.Level, ts time.Time, msg string) error {
	return l.stream.Send(&protocol.RunFixtureResponse{
		Control:   &protocol.RunFixtureResponse_Log{Log: msg},
		Timestamp: timestamppb.New(ts),
		Level:     protocol.LevelToProto(level),
	})
}

func (l *fixtureServiceLogger) Error(e *protocol.Error) error {
	return l.stream.Send(&protocol.RunFixtureResponse{
		Control: &protocol.RunFixtureResponse_Error{
			Error: &protocol.RunFixtureError{
				Reason: e.GetReason(),
				File:   e.GetLocation().GetFile(),
				Line:   int32(e.GetLocation().GetLine()),
				Stack:  e.GetLocation().GetStack(),
			},
		},
		Timestamp: timestamppb.Now(),
	})
}

func (l *fixtureServiceLogger) EntityStart(ei *protocol.Entity, outDir string) error {
	return nil
}

func (l *fixtureServiceLogger) EntityLog(ei *protocol.Entity, level logging.Level, ts time.Time, msg string) error {
	return l.Log(level, ts, msg)
}

func (l *fixtureServiceLogger) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	return l.Error(e)
}

func (l *fixtureServiceLogger) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	return nil
}

func (l *fixtureServiceLogger) ExternalEvent(req *protocol.RunTestsResponse) error {
	return nil
}

func (l *fixtureServiceLogger) StackOperation(ctx context.Context, req *protocol.StackOperationRequest) (*protocol.StackOperationResponse, error) {
	return nil, nil
}
