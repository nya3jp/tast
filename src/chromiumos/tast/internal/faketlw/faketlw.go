// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package faketlw provides a fake implementation of the TLW service.
package faketlw

import (
	"context"
	"fmt"
	"net"
	"testing"

	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"
)

// NamePort represents a simple name/port pair.
type NamePort struct {
	Name string
	Port int32
}

type wiringServerConfig struct {
	dutPortMap map[NamePort]NamePort
}

// WiringServerOption is an option passed to NewWiringServer to customize WiringServer.
type WiringServerOption func(cfg *wiringServerConfig)

// WithDUTPortMap returns an option that sets the name/port map used to resolve
// OpenDutPort requests.
func WithDUTPortMap(m map[NamePort]NamePort) WiringServerOption {
	return func(cfg *wiringServerConfig) {
		cfg.dutPortMap = m
	}
}

// WiringServer is a fake implementation of tls.WiringServer.
type WiringServer struct {
	tls.UnimplementedWiringServer
	cfg wiringServerConfig
}

var _ tls.WiringServer = &WiringServer{}

// NewWiringServer constructs a new WiringServer from given options.
func NewWiringServer(opts ...WiringServerOption) *WiringServer {
	var cfg wiringServerConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return &WiringServer{cfg: cfg}
}

// OpenDutPort implements tls.WiringServer.OpenDutPort.
func (s *WiringServer) OpenDutPort(ctx context.Context, req *tls.OpenDutPortRequest) (*tls.OpenDutPortResponse, error) {
	src := NamePort{Name: req.GetName(), Port: req.GetPort()}
	dst, ok := s.cfg.dutPortMap[src]
	if !ok {
		return nil, fmt.Errorf("not found in DUT port map: %s:%d", src.Name, src.Port)
	}
	return &tls.OpenDutPortResponse{Address: dst.Name, Port: dst.Port}, nil
}

// StartWiringServer is a convenient method for unit tests which starts a gRPC
// server serving WiringServer in the background.
// Callers are responsible for stopping the server by srv.Stop.
func StartWiringServer(t *testing.T, opts ...WiringServerOption) (srv *grpc.Server, addr string) {
	ws := NewWiringServer(opts...)

	srv = grpc.NewServer()
	tls.RegisterWiringServer(srv, ws)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go srv.Serve(lis)

	return srv, lis.Addr().String()
}
