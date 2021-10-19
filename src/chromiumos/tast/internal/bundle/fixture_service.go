// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

// fixtureService implements FixtureServiceServer gRPC service.
type fixtureService struct {
	reg *testing.Registry
}

var _ FixtureServiceServer = (*fixtureService)(nil)

// registerFixtureService registers fixture service.
func registerFixtureService(srv *grpc.Server, reg *testing.Registry) {
	RegisterFixtureServiceServer(srv, &fixtureService{reg: reg})
}

// RunFixture provides operations to set up and tear down fixtures.
// It accepts multiple pairs of push and pop requests in a loop until client
// closes the connection.
func (s *fixtureService) RunFixture(srv FixtureService_RunFixtureServer) error {
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
func (s *fixtureService) pushAndPop(srv FixtureService_RunFixtureServer) (retErr error) {
	ctx := srv.Context()

	sendDone := func() error {
		return srv.Send(&RunFixtureResponse{
			Control: &RunFixtureResponse_RequestDone{
				RequestDone: &empty.Empty{},
			},
			Timestamp: ptypes.TimestampNow(),
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
	dt, err := connectToTarget(ctx, r.Config.ConnectionSpec, r.Config.KeyFile, r.Config.KeyDir, nil)
	if err != nil {
		return fmt.Errorf("push %v: %v", r.Name, err)
	}
	defer func() {
		logging.Info(ctx, "Disconnecting from DUT")
		// It is OK to ignore the error since we've finished running fixture at this point.
		dt.Close(ctx)
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
		Features: &protocol.Features{
			Infra: &protocol.InfraFeatures{
				Vars: r.Config.TestVars,
			},
			Dut: &protocol.DUTFeatures{
				Software: &protocol.SoftwareFeatures{
					Available:   r.Config.AvailableSoftwareFeatures,
					Unavailable: r.Config.UnavailableSoftwareFeatures,
				},
				// TODO(oka): fill HardwareFeatures field.
			},
		},
	}
	if r.Config.CustomGracePeriod != nil {
		d, err := ptypes.Duration(r.Config.CustomGracePeriod)
		if err != nil {
			return fmt.Errorf("push %v: invalid CustomGracePeriod: %v", r.Name, err)
		}
		pcfg.CustomGracePeriod = &d
	}

	stack := planner.NewFixtureStack(pcfg, &fixtureServiceLogger{srv})
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
	stream FixtureService_RunFixtureServer
}

func (l *fixtureServiceLogger) Log(msg string) error {
	return l.stream.Send(&RunFixtureResponse{
		Control:   &RunFixtureResponse_Log{Log: msg},
		Timestamp: ptypes.TimestampNow(),
	})
}

func (l *fixtureServiceLogger) Error(e *protocol.Error) error {
	return l.stream.Send(&RunFixtureResponse{
		Control: &RunFixtureResponse_Error{
			Error: &RunFixtureError{
				Reason: e.GetReason(),
				File:   e.GetLocation().GetFile(),
				Line:   int32(e.GetLocation().GetLine()),
				Stack:  e.GetLocation().GetStack(),
			},
		},
		Timestamp: ptypes.TimestampNow(),
	})
}

func (l *fixtureServiceLogger) EntityStart(ei *protocol.Entity, outDir string) error {
	return nil
}

func (l *fixtureServiceLogger) EntityLog(ei *protocol.Entity, msg string) error {
	return l.Log(msg)
}

func (l *fixtureServiceLogger) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	return l.Error(e)
}

func (l *fixtureServiceLogger) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	return nil
}
