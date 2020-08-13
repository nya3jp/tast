// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"chromiumos/tast/dut"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/testing"
	"fmt"
)

// FixtureService implements FixtureService gRPC service.
type FixtureService struct {
	cancel map[string]func()
}

var _ bundle.FixtureServiceServer = (*FixtureService)(nil)

type Logger struct {
	stream bundle.FixtureService_RunServer
}

func (l *Logger) Log(msg string) error {
	return l.stream.Send(&bundle.RunResponse{
		Control: &bundle.RunResponse_Log{Log: msg},
	})
}

func (l *Logger) Error(e *testing.Error) error {
	return l.stream.Send(&bundle.RunResponse{
		Control: &bundle.RunResponse_Error{
			Error: &bundle.Error{
				Reason: e.Reason,
				File:   e.File,
				Line:   int32(e.Line),
				Stack:  e.Stack,
			},
		},
	})
}

func (s *FixtureService) Run(req *bundle.RunRequest, stream bundle.FixtureService_RunServer) error {
	ctx := stream.Context()

	// Retrieve the fixture instance.
	f := testing.GlobalRegistry().Fixture(req.Name)
	if f == nil {
		return fmt.Errorf("fixture %v not found", req.Name)
	}

	// Create DUT connection.
	dt, err := dut.New(req.Config.Target, req.Config.KeyFile, req.Config.KeyDir)
	if err != nil {
		return fmt.Errorf("failed to create DUT: %v", err)
	}
	defer dt.Close(ctx)

	// TODO(oka): Set up temp dir with req.Config.TempDir.

	// Create planner.Config from req.
	pcfg := &planner.Config{
		DataDir:           req.Config.DataDir,
		OutDir:            req.Config.OutDir,
		Vars:              req.Config.TestVars,
		Devservers:        req.Config.Devservers,
		BuildArtifactsURL: req.Config.BuildArtifactsUrl,

		RemoteData: &testing.RemoteData{
			// Meta field won't be used.
			RPCHint: &testing.RPCHint{
				LocalBundleDir: req.Config.LocalBundleDir,
			},
			DUT: dt,
		},
		// TODO(oka): consider supporting lazy downloading.
	}

	// Create testing.OutputStream from stream.
	lg := Logger{stream}

	// TODO(oka): Run fixture method.
	_, _ = lg, pcfg

	return nil
}
