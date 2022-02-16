// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"

	"go.chromium.org/chromiumos/config/go/longrunning"
	"go.chromium.org/chromiumos/config/go/test/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/errors"
)

// DUTServiceClient is an implementation of Client to communicate with DUT Service API.
type DUTServiceClient struct {
	dutServer string
	destDir   string
	conn      *grpc.ClientConn
}

var _ Client = &DUTServiceClient{}

// NewDUTServiceClient creates a DUTServiceClient.
func NewDUTServiceClient(ctx context.Context, dutServer string) (*DUTServiceClient, error) {
	destDir, err := ioutil.TempDir("/tmp", "dut_service_client_")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create temporary directory for downloading file %s", destDir)
	}
	conn, err := grpc.Dial(dutServer, grpc.WithInsecure())
	if err != nil {
		os.RemoveAll(destDir)
		return nil, errors.Wrapf(err, "failed to establish connection to DUT server: %s", dutServer)
	}
	return &DUTServiceClient{
		dutServer: dutServer,
		destDir:   destDir,
		conn:      conn,
	}, nil
}

// TearDown closes the gRPC connection to the DUTService service.
func (c *DUTServiceClient) TearDown() error {
	os.RemoveAll(c.destDir)
	return c.conn.Close()
}

// Stage downloads a file on GCS from storage.googleapis.com to specified destination
// and return file: url for the file.
func (c *DUTServiceClient) Stage(ctx context.Context, gsURL string) (*url.URL, error) {
	// verify GS URL format.
	_, path, err := ParseGSURL(gsURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse GS URL: %s", gsURL)
	}
	fullPath := filepath.Join(c.destDir, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create temporary dir %s", dir)
	}
	cl := api.NewDutServiceClient(c.conn)
	op, err := cl.Cache(ctx, &api.CacheRequest{
		Destination: &api.CacheRequest_File{
			File: &api.CacheRequest_LocalFile{
				Path: fullPath,
			},
		},
		Source: &api.CacheRequest_GsFile{
			GsFile: &api.CacheRequest_GSFile{
				SourcePath: gsURL},
		},
	})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return nil, errors.Wrap(os.ErrNotExist, gsURL)
		}
		return nil, errors.Wrap(err, "failed to call Cache")
	}

	opcli := longrunning.NewOperationsClient(c.conn)
	op, err = opcli.WaitOperation(ctx, &longrunning.WaitOperationRequest{
		Name: op.GetName(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait operation")
	}
	if !op.GetDone() {
		return nil, errors.Errorf("WaitOperation timed out (%v)", op)
	}

	if status := op.GetError(); status != nil {
		return nil, errors.Errorf("failed to download %s from cache server %s", gsURL, status.Message)
	}
	return &url.URL{
		Scheme: "file",
		Path:   fullPath,
	}, nil
}

// Open downloads a file on GCS from storage.googleapis.com to specified destination
// and return io.ReadCloser for the file.
func (c *DUTServiceClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	fileURL, err := c.Stage(ctx, gsURL)
	if err != nil {
		return nil, err
	}
	if fileURL.Scheme != "file" {
		return nil, errors.Errorf("Expected file url, got %q", fileURL)
	}
	file, err := os.Open(fileURL.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open temporary file %q", fileURL.Path)
	}
	return file, nil
}
