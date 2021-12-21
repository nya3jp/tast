// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/api/test/tls/dependencies/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/errors"
)

// TLWClient is an implementation of Client to communicate with Test Lab Services wiring API.
type TLWClient struct {
	dutName string
	conn    *grpc.ClientConn
}

var _ Client = &TLWClient{}

// NewTLWClient creates a TLWClient.
func NewTLWClient(ctx context.Context, tlwserver, dutName string) (*TLWClient, error) {
	conn, err := grpc.Dial(tlwserver, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to establish connection to server: %s", tlwserver)
	}
	return &TLWClient{
		dutName: dutName,
		conn:    conn,
	}, nil
}

// TearDown closes the gRPC connection to the TLW service.
func (c *TLWClient) TearDown() error {
	return c.conn.Close()
}

// Stage downloads a file on GCS from storage.googleapis.com using the TLW API.
func (c *TLWClient) Stage(ctx context.Context, gsURL string) (*url.URL, error) {
	// verify GS URL format.
	if _, _, err := ParseGSURL(gsURL); err != nil {
		return nil, errors.Wrapf(err, "failed to parse GS URL: %s", gsURL)
	}

	req := tls.CacheForDutRequest{Url: gsURL, DutName: c.dutName}
	cl := tls.NewWiringClient(c.conn)
	op, err := cl.CacheForDut(ctx, &req)
	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			return nil, errors.Wrapf(err, "failed to get status code")
		}
		if st.Code() == codes.NotFound {
			return nil, errors.Wrap(os.ErrNotExist, gsURL)
		}
		return nil, errors.Wrapf(err, "failed to call CacheForDut(%v)", req)
	}

	opcli := longrunning.NewOperationsClient(c.conn)
	op, err = opcli.WaitOperation(ctx, &longrunning.WaitOperationRequest{
		Name: op.GetName(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait operation")
	}
	if !op.GetDone() {
		return nil, fmt.Errorf("WaitOperation timed out (%v)", op)
	}

	resp := &tls.CacheForDutResponse{}
	if err := ptypes.UnmarshalAny(op.GetResponse(), resp); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal response: %v", resp)
	}
	return url.Parse(resp.Url)
}

// Open downloads a file on GCS from storage.googleapis.com using the TLW API.
func (c *TLWClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	url, err := c.Stage(ctx, gsURL)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new HTTP request: %s", url)
	}
	httpReq = httpReq.WithContext(ctx)

	res, err := defaultHTTPClient.Do(httpReq)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get from download URL: %s", url)
	}

	switch res.StatusCode {
	case http.StatusOK:
		return res.Body, nil
	case http.StatusNotFound:
		res.Body.Close()
		return nil, os.ErrNotExist
	default:
		res.Body.Close()
		return nil, fmt.Errorf("got status %d %v", res.StatusCode, httpReq)
	}
}
