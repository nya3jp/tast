// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakedutserver provides a fake implementation of the DUT service.
package fakedutserver

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/chromiumos/config/go/longrunning"
	"go.chromium.org/chromiumos/config/go/test/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type config struct {
	cacheFileMap map[string][]byte
}

// Option is an option passed to New to customize DutServiceServer.
type Option func(cfg *config)

// WithCacheFileMap returns an option that sets the files to be fetched by
// Cache requests.
func WithCacheFileMap(m map[string][]byte) Option {
	return func(cfg *config) {
		cfg.cacheFileMap = m
	}
}

type operation struct {
}

type operationsMap map[string]operation

// DutServiceServer is a fake implementation of api.DutService and
// longrunning.UnimplementedOperationsServer for Cache.
type DutServiceServer struct {
	api.UnimplementedDutServiceServer
	longrunning.UnimplementedOperationsServer
	cfg config

	operationsMu sync.RWMutex
	operations   operationsMap
}

var _ api.DutServiceServer = &DutServiceServer{}

// New constructs a new DutServiceServer from given options.
// The caller is responsible for calling Shutdown() of the returned object.
func New(opts ...Option) *DutServiceServer {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	return &DutServiceServer{
		cfg:        cfg,
		operations: map[string]operation{},
	}
}

// ExecCommand implements api.DutServiceServer.ExecCommand.
func (s *DutServiceServer) ExecCommand(req *api.ExecCommandRequest, stream api.DutService_ExecCommandServer) error {
	return status.Error(codes.Unimplemented, "not implemented")
}

// FetchCrashes implements api.DutServiceServer.FetchCrashes.
func (s *DutServiceServer) FetchCrashes(req *api.FetchCrashesRequest, stream api.DutService_FetchCrashesServer) error {
	return status.Error(codes.Unimplemented, "not implemented")
}

// Restart implements api.DutServiceServer.FetchCrashes.
func (s *DutServiceServer) Restart(ctx context.Context, req *api.RestartRequest) (*longrunning.Operation, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// DetectDeviceConfigId implements api.DutServiceServer.DetectDeviceConfigId.
func (s *DutServiceServer) DetectDeviceConfigId(
	req *api.DetectDeviceConfigIdRequest,
	stream api.DutService_DetectDeviceConfigIdServer) error {
	return status.Error(codes.Unimplemented, "not implemented")
}

// Cache implements api.DutServiceServer.Cache
func (s *DutServiceServer) Cache(ctx context.Context, req *api.CacheRequest) (*longrunning.Operation, error) {
	srcPath := ""
	if gsFile := req.GetGsFile(); gsFile != nil {
		srcPath = gsFile.GetSourcePath()
	} else if gsTarFile := req.GetGsTarFile(); gsTarFile != nil {
		return nil, status.Error(codes.Unimplemented, "support for tar file is not implemented")
	} else if gsZipFile := req.GetGsZipFile(); gsZipFile != nil {
		return nil, status.Error(codes.Unimplemented, "support for zip file is not implemented")
	} else {
		return nil, status.Errorf(codes.Unknown, "unknown file type: %v", req.Source)
	}
	content, ok := s.cfg.cacheFileMap[srcPath]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "not found in cache file map: %s", srcPath)
	}

	if err := fillCache(content, req.GetFile().Path); err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to create file %s: %v", req.GetFile().Path, err)
	}
	operationName := fmt.Sprintf("CacheOperation_%s", srcPath)
	s.beginOperation(operationName, srcPath)
	op := longrunning.Operation{
		Name: operationName,
		// Pretend operation is not finished yet in order to test the code path for
		// waiting the operation to finish.
		Done: false,
	}

	return &op, nil
}

func parseGSURL(gsURL string) (bucket, path string, err error) {
	parsed, err := url.Parse(gsURL)
	if err != nil {
		return "", "", err
	}
	if parsed.Scheme != "gs" {
		return "", "", fmt.Errorf("%s isnot a GS URL", gsURL)
	}

	bucket = parsed.Host
	path = strings.TrimPrefix(parsed.Path, "/")
	return bucket, path, nil
}

func (s *DutServiceServer) beginOperation(name, srcURL string) {
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()
	s.operations[name] = operation{}
}

func (s *DutServiceServer) operation(name string) (oper *operation, exists bool) {
	s.operationsMu.RLock()
	defer s.operationsMu.RUnlock()
	o, ok := s.operations[name]
	return &o, ok
}

func fillCache(content []byte, dest string) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return status.Errorf(codes.Internal, "failed to create directory %s: %v", dir, err)
	}
	if err := ioutil.WriteFile(dest, content, 0644); err != nil {
		return status.Errorf(codes.Internal, "failed to write file %s: %v", dest, err)
	}
	return nil
}

// GetOperation implements longrunning.GetOperation.
func (s *DutServiceServer) GetOperation(ctx context.Context, req *longrunning.GetOperationRequest) (*longrunning.Operation, error) {
	return s.finishOperation(req.Name)
}

// WaitOperation implements longrunning.WaitOperation.
func (s *DutServiceServer) WaitOperation(ctx context.Context, req *longrunning.WaitOperationRequest) (*longrunning.Operation, error) {
	return s.finishOperation(req.Name)
}

// CancelOperation implements longrunning.CancelOperation.
func (s *DutServiceServer) CancelOperation(ctx context.Context, req *longrunning.CancelOperationRequest) (*empty.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// DeleteOperation implements longrunning.CancelOperation.
func (s *DutServiceServer) DeleteOperation(ctx context.Context, req *longrunning.DeleteOperationRequest) (*empty.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// ListOperations implements longrunning.ListOperations.
func (s *DutServiceServer) ListOperations(ctx context.Context, req *longrunning.ListOperationsRequest) (*longrunning.ListOperationsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *DutServiceServer) finishOperation(name string) (*longrunning.Operation, error) {
	_, ok := s.operation(name)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "failed to find operation: %s", name)
	}
	m, err := ptypes.MarshalAny(&api.CacheResponse{Result: &api.CacheResponse_Success_{}})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data: %s", err)
	}
	return &longrunning.Operation{
		Done: true,
		Name: name,
		Result: &longrunning.Operation_Response{
			Response: m,
		},
	}, nil
}

// ForceReconnect implements api.ForceReconnect.
func (s *DutServiceServer) ForceReconnect(ctx context.Context, req *api.ForceReconnectRequest) (*longrunning.Operation, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// Start is a convenient method for unit tests which starts a gRPC
// server serving DutServiceServer in the background. It also starts an HTTP server
// for serving cached files by Cache.
// Callers are responsible for stopping the server by stopFunc().
func Start(t *testing.T, opts ...Option) (stopFunc func(), addr string) {
	ws := New(opts...)

	srv := grpc.NewServer()
	api.RegisterDutServiceServer(srv, ws)
	longrunning.RegisterOperationsServer(srv, ws)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go srv.Serve(lis)

	return func() {
		srv.Stop()
	}, lis.Addr().String()
}
