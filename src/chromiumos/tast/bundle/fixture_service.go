// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"chromiumos/tast/dut"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"

	"fmt"

	"github.com/golang/protobuf/ptypes"
)

// FixtureService implements FixtureService gRPC service.
type FixtureService struct {
}

var _ bundle.FixtureServiceServer = (*FixtureService)(nil)

type Logger struct {
	stream bundle.FixtureService_RunFixtureServer
}

func (l *Logger) Log(msg string) error {
	return l.stream.Send(&bundle.RunFixtureResponse{
		Control:   &bundle.RunFixtureResponse_Log{Log: msg},
		Timestamp: ptypes.TimestampNow(),
	})
}

func (l *Logger) Error(e *testing.Error) error {
	return l.stream.Send(&bundle.RunFixtureResponse{
		Control: &bundle.RunFixtureResponse_Error{
			Error: &bundle.RunFixtureError{
				Reason: e.Reason,
				File:   e.File,
				Line:   int32(e.Line),
				Stack:  e.Stack,
			},
		},
		Timestamp: ptypes.TimestampNow(),
	})
}

func (l *Logger) EntityStart(ei *testing.EntityInfo, outDir string) error {
	return nil
}

func (l *Logger) EntityLog(ei *testing.EntityInfo, msg string) error {
	return l.Log(msg)
}

func (l *Logger) EntityError(ei *testing.EntityInfo, e *testing.Error) error {
	return l.Error(e)
}

func (l *Logger) EntityEnd(ei *testing.EntityInfo, skipReasons []string, timingLog *timing.Log) error {
	return nil
}

func (s *FixtureService) RunFixture(srv bundle.FixtureService_RunFixtureServer) error {
	// ctx has the same lifetime as the fixture to run.
	ctx := srv.Context()

	var state *planner.FixtState
	for {
		req, err := srv.Recv()
		if err != nil {
			return err
		}
		switch v := req.Control.(type) {
		case *bundle.RunFixtureRequest_SetUp:
			if state != nil {
				return fmt.Errorf("SetUp called twice")
			}
			r := v.SetUp
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

			// Create planner.Config from r.
			pcfg := &planner.Config{
				DataDir: r.Config.DataDir,
				OutDir:  r.Config.OutDir,
				Vars:    r.Config.TestVars,
				// Devservers:        r.Config.Devservers,
				// BuildArtifactsURL: r.Config.BuildArtifactsUrl,

				RemoteData: &testing.RemoteData{
					// Meta field won't be used.
					RPCHint: &testing.RPCHint{
						LocalBundleDir: r.Config.LocalBundleDir,
					},
					DUT: dt,
				},
				// TODO(oka): consider supporting lazy downloading.
			}
			// Create testing.OutputStream from stream.
			lg := &Logger{srv}

			state, err = planner.SetUpFixture(ctx, r.Name, lg, pcfg)
			if err != nil {
				srv.Send(&bundle.RunFixtureResponse{
					Control:   &bundle.RunFixtureResponse_Error{Error: &bundle.RunFixtureError{Reason: err.Error()}},
					Timestamp: ptypes.TimestampNow(),
				})
				return nil
			}
			// SetUp Done.
			if err := srv.Send(&bundle.RunFixtureResponse{
				Control: &bundle.RunFixtureResponse_SetUpDone{
					SetUpDone: &bundle.RunFixtureSetUpDone{},
				},
				Timestamp: ptypes.TimestampNow(),
			}); err != nil {
				return err
			}
		case *bundle.RunFixtureRequest_TearDown:
			r := v.TearDown
			if r.Token == nil {
				return fmt.Errorf("TearDown request without valid token")
			}
			if state == nil {
				return fmt.Errorf("TearDown request in invalid state")
			}
			return planner.TearDownFixture(ctx, state)
		}
	}
}
