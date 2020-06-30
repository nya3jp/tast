// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type server struct {
	clArgs []string
	args   *Args
	cfg    *Config
	stderr io.Writer
}

var _ TastCoreServiceServer = (*server)(nil)
var _ LocalRunnerServiceServer = (*server)(nil)

type delegateSender struct {
	stream TastCoreService_DelegateServer
}

func (d *delegateSender) Write(b []byte) (int, error) {
	return len(b), d.stream.Send(&DelegateResponse{
		Payload: string(b),
	})
}

func (s *server) Delegate(req *DelegateRequest, stream TastCoreService_DelegateServer) error {
	w := &delegateSender{stream}
	r := strings.NewReader(req.Payload)
	if status := Run(s.clArgs, r, w, s.stderr, s.args, s.cfg); status != statusSuccess {
		return fmt.Errorf("runner failed with status %d", status)
	}
	return nil
}

func (s *server) DUTInfo(ctx context.Context, req *DUTInfoRequest) (*DUTInfoResponse, error) {
	s.args.GetDUTInfo = &GetDUTInfoArgs{
		ExtraUSEFlags:       req.ExtraUseFlags,
		RequestDeviceConfig: req.RequestDeviceConfig,
	}
	res, err := dutInfo(s.args, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("dutInfo: %w", err)
	}
	return &DUTInfoResponse{
		SoftwareFeatures: &SoftwareFeatures{
			Available:   res.SoftwareFeatures.Available,
			Unavailable: res.SoftwareFeatures.Unavailable,
		},
		DeviceConfig: res.DeviceConfig,
		Warnings:     res.Warnings,
	}, nil
}

func (s *server) SysInfoState(ctx context.Context, req *SysInfoStateRequest) (*GetSysInfoStateResult, error) {
	return sysInfoState(ctx, s.cfg)
}

func (s *server) CollectSysInfo(ctx context.Context, req *CollectSysInfoArgs) (*CollectSysInfoResult, error) {
	return collectSysInfo(ctx, req, s.cfg)
}
