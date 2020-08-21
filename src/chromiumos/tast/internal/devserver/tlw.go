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
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/api/test/tls/dependencies/longrunning"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
)

// TLWClient is an implementation of Client to communicate with Test Lab wiring service.
type TLWClient struct {
	server         string
	cl             *http.Client
	baseURL        string
	dutName        string
	opPollInterval time.Duration
	conn           *grpc.ClientConn
}

var _ Client = &TLWClient{}

type tlwClientOptions struct {
	cl             *http.Client
	baseURL        string
	dutName        string
	opPollInterval time.Duration
}

// TLWClientOption is an option accepted by NewPseudoClient to configure
// TLWClient initialization.
type TLWClientOption func(o *tlwClientOptions)

// WithDutName returns an option that specifies the base URL of Google Cloud
// Storage HTTP API.
func WithDutName(baseURL string) PseudoClientOption {
	return func(o *pseudoClientOptions) { o.baseURL = baseURL }
}

// NewTLWClient creates a TLWClient.
func NewTLWClient(tlsserver string, opts ...TLWClientOption) *TLWClient {
	o := &tlwClientOptions{
		cl:             defaultHTTPClient,
		baseURL:        gsDownloadURL,
		dutName:        "",
		opPollInterval: 5 * time.Second,
	}
	for _, opts := range opts {
		opts(o)
	}
	return &TLWClient{
		server:         tlsserver,
		cl:             o.cl,
		baseURL:        o.baseURL,
		opPollInterval: o.opPollInterval,
		dutName:        o.dutName,
	}
}

// Open downloads a file on GCS directly from storage.googleapis.com.
func (c *TLWClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	// verify GS URL format.
	_, _, err := ParseGSURL(gsURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse GS URL: %s", gsURL)
	}

	if c.conn == nil {
		// TODO: whether to apply WithInsecure when unit-testing
		var err error
		c.conn, err = grpc.Dial(c.server, grpc.WithInsecure())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to establish connection to server: %s", c.server)
		}
	}

	req := tls.CacheForDutRequest{Url: gsURL, DutName: c.dutName}
	cl := tls.NewWiringClient(c.conn)
	op, err := cl.CacheForDut(ctx, &req)
	if err != nil {
		// TODO: check what RPC error returned by TLW CacheForDut in case of file not found and else.
		return nil, os.ErrNotExist
		// return nil, errors.Wrapf(err, "failed to call CacheForDut(%v)", req)
	}

	opcli := longrunning.NewOperationsClient(c.conn)
	for {
		if op.GetDone() {
			break
		}
		logging.ContextLogf(ctx, "still waiting")
		time.Sleep(c.opPollInterval)
		op, err = opcli.GetOperation(ctx, &longrunning.GetOperationRequest{
			Name: op.GetName(),
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get operation")
		}
	}

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
		return nil, fmt.Errorf("%v %v", resp, req2)
		return nil, os.ErrNotExist
	default:
		res.Body.Close()
		return nil, fmt.Errorf("got status %d %v", res.StatusCode, req2)
	}
}
