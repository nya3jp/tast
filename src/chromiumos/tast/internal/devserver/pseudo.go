// Copyright 2018 The Chromium OS Authors. All rights reserved.
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
)

const gsDownloadURL = "https://storage.googleapis.com"

// PseudoClient is an implementation of Client to simulate devservers without credentials.
type PseudoClient struct {
	cl      *http.Client
	baseURL string
}

var _ Client = &PseudoClient{}

type pseudoClientOptions struct {
	cl      *http.Client
	baseURL string
}

// PseudoClientOption is an option accepted by NewPseudoClient to configure
// PseudoClient initialization.
type PseudoClientOption func(o *pseudoClientOptions)

// WithHTTPClient returns an option that specifies http.Client used by
// PseudoClient.
func WithHTTPClient(cl *http.Client) PseudoClientOption {
	return func(o *pseudoClientOptions) { o.cl = cl }
}

// WithBaseURL returns an option that specifies the base URL of Google Cloud
// Storage HTTP API.
func WithBaseURL(baseURL string) PseudoClientOption {
	return func(o *pseudoClientOptions) { o.baseURL = baseURL }
}

// NewPseudoClient creates a PseudoClient.
func NewPseudoClient(opts ...PseudoClientOption) *PseudoClient {
	o := &pseudoClientOptions{
		cl:      defaultHTTPClient,
		baseURL: gsDownloadURL,
	}
	for _, opts := range opts {
		opts(o)
	}
	return &PseudoClient{cl: o.cl, baseURL: o.baseURL}
}

// TearDown does nothing.
func (c *PseudoClient) TearDown() error {
	return nil
}

// Stage returns a url to GCS directly from storage.googleapis.com.
func (c *PseudoClient) Stage(ctx context.Context, gsURL string) (*url.URL, error) {
	bucket, path, err := ParseGSURL(gsURL)
	if err != nil {
		return nil, err
	}

	dlURL, _ := url.Parse(c.baseURL)
	dlURL.Path = fmt.Sprintf("/%s/%s", bucket, path)
	return dlURL, nil
}

// Open downloads a file on GCS directly from storage.googleapis.com.
func (c *PseudoClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	dlURL, err := c.Stage(ctx, gsURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", dlURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return nil, err
	}

	switch res.StatusCode {
	case http.StatusOK:
		return res.Body, nil
	case http.StatusNotFound:
		res.Body.Close()
		return nil, os.ErrNotExist
	default:
		res.Body.Close()
		return nil, fmt.Errorf("got status %d", res.StatusCode)
	}
}
