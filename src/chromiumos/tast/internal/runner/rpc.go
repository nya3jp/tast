// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"path/filepath"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/ssh"
)

func runRPCServer(r io.Reader, w io.Writer) error {
	return rpc.RunServer(r, w, nil, func(srv *grpc.Server) {
		protocol.RegisterTestServiceServer(srv, &testService{})
	})
}

type testService struct {
}

func (s *testService) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	var es []*protocol.Entity

	// Enumerate entities on the target machine if it exists.
	if target := req.GetDeviceConfig().GetTargetDevice(); target != nil {
		tes, err := listTargetEntities(ctx, target)
		if err != nil {
			return nil, err
		}
		es = append(es, tes...)
	}

	// Enumerate entities on this machine.
	hes, err := listHostEntities(ctx, req.GetDeviceConfig())
	if err != nil {
		return nil, err
	}
	es = append(es, hes...)

	return &protocol.ListEntitiesResponse{Entities: es}, nil
}

func (s *testService) RunTests(req *protocol.RunTestsRequest, srv protocol.TestService_RunTestsServer) error {
}

func listHostEntities(ctx context.Context, layout *protocol.DeviceConfig) ([]*protocol.Entity, error) {
	bundlePaths, err := filepath.Glob(layout.GetBundlePathGlob())
	if err != nil {
		return nil, err
	}

	var es []*protocol.Entity
	for _, bundlePath := range bundlePaths {
		if err := func() error {
			cl, err := rpc.DialLocal(ctx, bundlePath, &protocol.HandshakeRequest{})
			if err != nil {
				return err
			}
			defer cl.Close(ctx)

			ts := protocol.NewTestServiceClient(cl.Conn)
			req := &protocol.ListEntitiesRequest{DeviceConfig: layout}
			res, err := ts.ListEntities(ctx, req)
			if err != nil {
				return err
			}

			es = append(es, res.GetEntities()...)
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return es, nil
}

func listTargetEntities(ctx context.Context, target *protocol.TargetDevice) ([]*protocol.Entity, error) {
	// TODO: Fill ssh.Options.
	conn, err := ssh.New(ctx, &ssh.Options{})
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cl, err := rpc.Dial(ctx, conn, target.GetDeviceConfig().GetRunnerPath(), &protocol.HandshakeRequest{})
	if err != nil {
		return nil, err
	}
	defer cl.Close(ctx)

	ts := protocol.NewTestServiceClient(cl.Conn)
	req := &protocol.ListEntitiesRequest{DeviceConfig: target.GetDeviceConfig()}
	res, err := ts.ListEntities(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.GetEntities(), nil
}
