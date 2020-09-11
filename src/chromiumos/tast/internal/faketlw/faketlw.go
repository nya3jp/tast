// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package faketlw provides a fake implementation of the TLW service.
package faketlw

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	dutPortMap   map[NamePort]NamePort
	cacheFileMap map[string][]byte
	dutName      string
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

// WithCacheFileMap returns an option that sets the files to be fetched by
// CacheForDUT requests.
func WithCacheFileMap(m map[string][]byte) WiringServerOption {
	return func(cfg *wiringServerConfig) {
		cfg.cacheFileMap = m
	}
}

// WithDutName returns an option that sets the expeted DUT name to be requested by
// CacheForDuT reqeusts.
func WithDutName(n string) WiringServerOption {
	return func(cfg *wiringServerConfig) {
		cfg.dutName = n
	}
}

// cacheForDutStatus stores progress and result of CacheForDUT, and shared by
// the fake Wiring server and the fake Operations server.
type cacheForDutStatus struct {
	cachedFiles   map[string][]byte
	operationsMap operationsMap
	address       string
}

type operation struct {
	name    string
	srcURL  string
	content []byte
}

type operationsMap map[string]operation

// WiringServer is a fake implementation of tls.WiringServer.
type WiringServer struct {
	tls.UnimplementedWiringServer
	cfg      wiringServerConfig
	status   cacheForDutStatus
	opServer operationsServer
	hs       *http.Server
}

var _ tls.WiringServer = &WiringServer{}

// NewWiringServer constructs a new WiringServer from given options.
// The caller is responsible for calling Shutdown() of the returned object.
func NewWiringServer(opts ...WiringServerOption) (*WiringServer, error) {
	var cfg wiringServerConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	ws := &WiringServer{
		cfg: cfg,
		status: cacheForDutStatus{
			cachedFiles:   map[string][]byte{},
			operationsMap: operationsMap{},
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(ws.handleHTTP))
	ws.hs = &http.Server{
		Handler: mux,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go ws.hs.Serve(listener)
	ws.status.address = listener.Addr().String()

	return ws, nil
}

// Shutdown shuts down the serer.
func (s *WiringServer) Shutdown() {
	s.hs.Shutdown(context.Background())
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
	k, err := cacheKey(r.URL.String())
	if err != nil {
		http.NotFound(w, r)
	}
	content, ok := s.status.cachedFiles[k]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write(content)
}

// CacheForDut implements tls WiringServer.CacheForDUT
func (s *WiringServer) CacheForDut(ctx context.Context, req *tls.CacheForDutRequest) (*longrunning.Operation, error) {
	if req.DutName != s.cfg.dutName {
		return nil, fmt.Errorf("wrong DUT name: got %q, want %q", req.DutName, s.cfg.dutName)
	}
	content, ok := s.cfg.cacheFileMap[req.Url]
	if !ok {
		return nil, fmt.Errorf("not found in cache file map: %s", req.Url)
	}

	operationName := fmt.Sprintf("CacheForDUTOperation_%s", req.Url)
	s.status.operationsMap[operationName] = operation{
		name:    operationName,
		srcURL:  req.Url,
		content: content,
	}
	op := longrunning.Operation{
		Name: operationName,
		// Pretend operation is not finished yet in order to test the code path for
		// waiting the operation to finish.
		Done: false,
	}

	return &op, nil
}

func (s *WiringServer) cacheForDutStatus() *cacheForDutStatus {
	return &s.status
}

// cacheURL composes a URL of a fake cached file.
func cacheURL(srvAddress, src string) (string, error) {
	u := fmt.Sprintf("http://%s/?s=%s", srvAddress, url.QueryEscape(src))
	p, err := url.Parse(u)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse URL: %s", u)
	}
	return p.String(), nil
}

// cacheKey generates the internal key used for matching a URL generated by CacheForDUT,
// and a one that is passed to the HTTP handler.
func cacheKey(cacheURL string) (string, error) {
	u, err := url.Parse(cacheURL)
	if err != nil {
		return "", err
	}

	// The query parameter name should be kept consistent with cacheURL.
	q := u.Query()["s"]
	if len(q) != 1 {
		return "", fmt.Errorf("failed to find query in cache URL %s", cacheURL)
	}
	return q[0], nil
}

// StartWiringServer is a convenient method for unit tests which starts a gRPC
// server serving WiringServer in the background. It also starts an HTTP server
// for serving cached files by CacheForDUT.
// Callers are responsible for stopping the server by stopFunc().
func StartWiringServer(t *testing.T, opts ...WiringServerOption) (stopFunc func(), addr string) {
	ws, err := NewWiringServer(opts...)
	if err != nil {
		t.Fatal("Failed to start new Wiring Server: ", err)
	}

	srv := grpc.NewServer()
	tls.RegisterWiringServer(srv, ws)

	ws.opServer = operationsServer{
		cacheForDutStatus: ws.cacheForDutStatus(),
	}
	longrunning.RegisterOperationsServer(srv, ws.opServer)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go srv.Serve(lis)

	return func() {
		ws.Shutdown()
		srv.Stop()
	}, lis.Addr().String()
}

// operationsServer is a fake implementation of longrunning.Operations.
type operationsServer struct {
	longrunning.UnimplementedOperationsServer
	cacheForDutStatus *cacheForDutStatus
}

// GetOperation implements longrunning.GetOperation.
func (s operationsServer) GetOperation(ctx context.Context, req *longrunning.GetOperationRequest) (*longrunning.Operation, error) {
	name := req.Name
	o, ok := s.cacheForDutStatus.operationsMap[name]
	if !ok {
		// TODO(yamaguchi): Check the exact error format returned by the real service.
		return nil, status.Errorf(codes.NotFound, "operation name %s not found", name)
	}

	cacheURL, err := cacheURL(s.cacheForDutStatus.address, o.srcURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate cache URL")
	}
	k, err := cacheKey(cacheURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate cache key: %s", cacheURL)
	}
	s.cacheForDutStatus.cachedFiles[k] = o.content
	m, err := ptypes.MarshalAny(&tls.CacheForDutResponse{Url: cacheURL})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal data in GetOperation")
	}
	return &longrunning.Operation{
		Done: true,
		Name: name,
		Result: &longrunning.Operation_Response{
			Response: m,
		},
	}, nil
}

// CancelOperation implements longrunning.CancelOperation.
func (s operationsServer) CancelOperation(ctx context.Context, req *longrunning.CancelOperationRequest) (*empty.Empty, error) {
	return nil, errors.New("Not implemented")
}

// DeleteOperation implements longrunning.CancelOperation.
func (s operationsServer) DeleteOperation(ctx context.Context, req *longrunning.DeleteOperationRequest) (*empty.Empty, error) {
	return nil, errors.New("Not implemented")
}

// ListOperations implements longrunning.ListOperations.
func (s operationsServer) ListOperations(ctx context.Context, req *longrunning.ListOperationsRequest) (*longrunning.ListOperationsResponse, error) {
	return nil, errors.New("Not implemented")
}
