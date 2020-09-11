// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package faketlw provides a fake implementation of the TLW service.
package faketlw

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/api/test/tls/dependencies/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/errors"
)

// NamePort represents a simple name/port pair.
type NamePort struct {
	Name string
	Port int32
}

type wiringServerConfig struct {
	dutPortMap       map[NamePort]NamePort
	cacheFileMap     map[string]string
	cacheHTTPAddress string
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

// WithCacheFileMap returns an option that sets the file URL map used to resolve
// CacheForDUT requests.
func WithCacheFileMap(m map[string]string) WiringServerOption {
	return func(cfg *wiringServerConfig) {
		cfg.cacheFileMap = m
	}
}

// WiringServer is a fake implementation of tls.WiringServer.
type WiringServer struct {
	tls.UnimplementedWiringServer
	cfg           wiringServerConfig
	cachedFiles   map[string]struct{}
	operationsMap *OperationsMap
}

var _ tls.WiringServer = &WiringServer{}

// NewWiringServer constructs a new WiringServer from given options.
func NewWiringServer(operationsMap *OperationsMap, opts ...WiringServerOption) *WiringServer {
	cfg := wiringServerConfig{
		cacheHTTPAddress: "127.0.0.1:2222",
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &WiringServer{
		cfg:           cfg,
		cachedFiles:   map[string]struct{}{},
		operationsMap: operationsMap,
	}
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

func (s *WiringServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cachedFiles[r.URL.Path]; !ok {
		http.NotFound(w, r)
		return
	}
	fmt.Fprintf(w, "This is data file.")
}

// CacheForDut implements tls WiringServer.CacheForDUT
func (s *WiringServer) CacheForDut(ctx context.Context, req *tls.CacheForDutRequest) (*longrunning.Operation, error) {
	cPath, ok := s.cfg.cacheFileMap[req.Url]
	if !ok {
		return nil, errors.New("RPC ERRR")
	}
	// TODO: compose URL from server address and requested file name
	s.cachedFiles["/"+cPath] = struct{}{}
	url := "http://" + s.cfg.cacheHTTPAddress + "/" + cPath
	m, err := ptypes.MarshalAny(&tls.CacheForDutResponse{Url: url})
	if err != nil {
		return nil, err
	}

	operationName := "test0001" // TODO: make unique
	(*s.operationsMap)[operationName] = operation{
		name:     operationName,
		cacheURL: url,
	}
	op := longrunning.Operation{
		Name: operationName,
		// Pretend operation is not finished yet in order to test the code path for
		// waiting the operation to finish.
		Done: false,
		Result: &longrunning.Operation_Response{
			Response: m,
		},
	}

	return &op, nil
}

// StartWiringServer is a convenient method for unit tests which starts a gRPC
// server serving WiringServer in the background. It also starts an HTTP server
// for serving cached files by CacheForDUT.
// Callers are responsible for stopping the server by stopFunc() when err != nil.
func StartWiringServer(t *testing.T, opts ...WiringServerOption) (stopFunc func(), addr string, retErr error) {
	om := &OperationsMap{}
	ws := NewWiringServer(om, opts...)

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(ws.handleHTTP))
	hs := &http.Server{
		Handler: mux,
	}
	listener, err := net.Listen("tcp", ws.cfg.cacheHTTPAddress)
	if err != nil {
		return nil, "", err
	}

	go func() {
		err := hs.Serve(listener)
		if err != nil {
			log.Println("HTTP Server Error - ", err)
		}
	}()

	srv := grpc.NewServer()
	tls.RegisterWiringServer(srv, ws)

	opServer := NewOperationsServer(om)
	longrunning.RegisterOperationsServer(srv, opServer)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go srv.Serve(lis)

	return func() {
		srv.Stop()
		hs.Shutdown(context.Background())
	}, lis.Addr().String(), nil
}

type operation struct {
	name     string
	cacheURL string
}

// OperationsMap stores started operations in the fake service.
type OperationsMap map[string]operation

// OperationsServer is a fake implementation of longrunning.Operations.
type OperationsServer struct {
	longrunning.UnimplementedOperationsServer
	operations *OperationsMap
}

// NewOperationsServer constructs a new OperationsServer from given options.
func NewOperationsServer(operations *OperationsMap) OperationsServer {
	return OperationsServer{
		operations: operations,
	}
}

// GetOperation implements longrunning.GetOperation.
func (s OperationsServer) GetOperation(ctx context.Context, req *longrunning.GetOperationRequest) (*longrunning.Operation, error) {
	name := req.Name
	o, ok := (*s.operations)[name]
	if !ok {
		// TODO: check what error code should be returned in this case
		return nil, status.Errorf(codes.NotFound, "operation name %s not found", name)
	}
	m, err := ptypes.MarshalAny(&tls.CacheForDutResponse{Url: o.cacheURL})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal data in GetOperation")
	}
	return &longrunning.Operation{
		Done: true,
		Name: "dummy",
		Result: &longrunning.Operation_Response{
			Response: m,
		},
	}, nil
}

// CancelOperation implements longrunning.CancelOperation.
func (s OperationsServer) CancelOperation(ctx context.Context, req *longrunning.CancelOperationRequest) (*empty.Empty, error) {
	return nil, nil
}

// DeleteOperation implements longrunning.CancelOperation.
func (s OperationsServer) DeleteOperation(ctx context.Context, req *longrunning.DeleteOperationRequest) (*empty.Empty, error) {
	return nil, nil
}

// ListOperations implements longrunning.ListOperations.
func (s OperationsServer) ListOperations(ctx context.Context, req *longrunning.ListOperationsRequest) (*longrunning.ListOperationsResponse, error) {
	return nil, nil
}
