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
	"net/http/httptest"
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

// WithDUTName returns an option that sets the expected DUT name to be requested by
// CacheForDut requests.
func WithDUTName(n string) WiringServerOption {
	return func(cfg *wiringServerConfig) {
		cfg.dutName = n
	}
}

type operation struct {
	srcURL string
}

type operationsMap map[string]operation

// WiringServer is a fake implementation of tls.WiringServer and
// longrunning.UnimplementedOperationsServer for CacheForDUT.
type WiringServer struct {
	tls.UnimplementedWiringServer
	longrunning.UnimplementedOperationsServer
	cfg         wiringServerConfig
	cacheServer *cacheServer
	operations  operationsMap
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
		cfg:        cfg,
		operations: map[string]operation{},
	}

	ws.cacheServer = newCacheServer()

	return ws, nil
}

// Shutdown shuts down the serer.
func (s *WiringServer) Shutdown() {
	s.cacheServer.hs.Close()
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

// CacheForDut implements tls WiringServer.CacheForDUT
func (s *WiringServer) CacheForDut(ctx context.Context, req *tls.CacheForDutRequest) (*longrunning.Operation, error) {
	if req.DutName != s.cfg.dutName {
		return nil, fmt.Errorf("wrong DUT name: got %q, want %q", req.DutName, s.cfg.dutName)
	}
	_, ok := s.cfg.cacheFileMap[req.Url]
	if !ok {
		return nil, fmt.Errorf("not found in cache file map: %s", req.Url)
	}

	operationName := fmt.Sprintf("CacheForDUTOperation_%s", req.Url)
	s.operations[operationName] = operation{
		srcURL: req.Url,
	}
	op := longrunning.Operation{
		Name: operationName,
		// Pretend operation is not finished yet in order to test the code path for
		// waiting the operation to finish.
		Done: false,
	}

	return &op, nil
}

func (s *WiringServer) fillCache(srcURL string) (string, error) {
	cacheURL := fmt.Sprintf("http://%s/?s=%s",
		s.cacheServer.address(), url.QueryEscape(srcURL))
	k, err := cacheKey(cacheURL)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate cache key: %s", cacheURL)
	}
	content, ok := s.cfg.cacheFileMap[srcURL]
	if !ok {
		return "", fmt.Errorf("requrested URL does not exist: %s", srcURL)
	}
	s.cacheServer.fillCache(k, content)
	return cacheURL, nil
}

// GetOperation implements longrunning.GetOperation.
func (s *WiringServer) GetOperation(ctx context.Context, req *longrunning.GetOperationRequest) (*longrunning.Operation, error) {
	name := req.Name
	o, ok := s.operations[name]
	if !ok {
		// TODO(yamaguchi): Check the exact error format returned by the real service.
		return nil, status.Errorf(codes.NotFound, "operation name %s not found", name)
	}
	cacheURL, err := s.fillCache(o.srcURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fill cache")
	}
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
func (s *WiringServer) CancelOperation(ctx context.Context, req *longrunning.CancelOperationRequest) (*empty.Empty, error) {
	return nil, errors.New("not implemented")
}

// DeleteOperation implements longrunning.CancelOperation.
func (s *WiringServer) DeleteOperation(ctx context.Context, req *longrunning.DeleteOperationRequest) (*empty.Empty, error) {
	return nil, errors.New("not implemented")
}

// ListOperations implements longrunning.ListOperations.
func (s *WiringServer) ListOperations(ctx context.Context, req *longrunning.ListOperationsRequest) (*longrunning.ListOperationsResponse, error) {
	return nil, errors.New("not implemented")
}

// cacheKey generates the internal key used for matching a URL generated by CacheForDUT,
// and a one that is passed to the HTTP handler.
func cacheKey(cacheURL string) (string, error) {
	u, err := url.Parse(cacheURL)
	if err != nil {
		return "", err
	}

	// The query parameter name should be kept consistent with WiringServer.fillCache().
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

	longrunning.RegisterOperationsServer(srv, ws)

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

type cacheServer struct {
	cachedFiles map[string][]byte
	hs          *httptest.Server
}

func newCacheServer() *cacheServer {
	c := cacheServer{
		cachedFiles: map[string][]byte{},
	}
	c.hs = httptest.NewServer(&c)
	return &c
}

func (c *cacheServer) fillCache(key string, content []byte) {
	c.cachedFiles[key] = content
}

func (c *cacheServer) address() string {
	return c.hs.Listener.Addr().String()
}

func (c *cacheServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	k, err := cacheKey(r.URL.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, ok := c.cachedFiles[k]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write(content)
}
