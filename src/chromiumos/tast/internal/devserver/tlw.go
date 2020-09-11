// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/api/test/tls/dependencies/longrunning"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
)

// TLWClient is an implementation of Client to communicate with Test Lab Services wiring API.
type TLWClient struct {
	cl      *http.Client
	dutName string
	conn    *grpc.ClientConn
}

var _ Client = &TLWClient{}

type tlwClientOptions struct {
	cl      *http.Client
	dutName string
}

// TLWClientOption is an option accepted by NewPseudoClient to configure
// TLWClient initialization.
type TLWClientOption func(o *tlwClientOptions)

// WithDutName returns an option that specifies the DUT name passed to the
// Test Lab Service wiring API.
func WithDutName(dutName string) TLWClientOption {
	return func(o *tlwClientOptions) { o.dutName = dutName }
}

// NewTLWClient creates a TLWClient.
func NewTLWClient(ctx context.Context, tlwserver string, opts ...TLWClientOption) (*TLWClient, error) {
	o := &tlwClientOptions{
		cl:      defaultHTTPClient,
		dutName: "",
	}
	for _, opts := range opts {
		opts(o)
	}
	conn, err := grpc.Dial(tlwserver, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to establish connection to server: %s", tlsserver)
	}
	return &TLWClient{
		cl:      o.cl,
		dutName: o.dutName,
		conn:    conn,
	}, nil
}

// Open downloads a file on GCS directly from storage.googleapis.com.
func (c *TLWClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	// verify GS URL format.
	if _, _, err := ParseGSURL(gsURL); err != nil {
		return nil, errors.Wrapf(err, "failed to parse GS URL: %s", gsURL)
	}

	req := tls.CacheForDutRequest{Url: gsURL, DutName: c.dutName}
	cl := tls.NewWiringClient(c.conn)
	op, err := cl.CacheForDut(ctx, &req)
	if err != nil {
		// TODO(crbug/1112616): check what RPC error returned by the actual TLW API.
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
		return nil, errors.New("Timed out")
	}
	return nil, errors.New("Timed out")

	resp := &tls.CacheForDutResponse{}
	if err := ptypes.UnmarshalAny(op.GetResponse(), resp); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal response: %v", resp)
	}
	req2, err := http.NewRequest("GET", resp.Url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new HTTP request: %s", resp.Url)
	}
	req2 = req2.WithContext(ctx)

	res, err := c.cl.Do(req2)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get from download URL: %s", resp.Url)
	}

	switch res.StatusCode {
	case http.StatusOK:
		return res.Body, nil
	case http.StatusNotFound:
		res.Body.Close()
		return nil, os.ErrNotExist
	default:
		res.Body.Close()
		return nil, fmt.Errorf("got status %d %v", res.StatusCode, req2)
	}
}
