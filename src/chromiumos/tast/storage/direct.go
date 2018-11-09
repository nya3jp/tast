// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// DirectClient is an implementation of Client to download files from GCS directly.
type DirectClient struct {
	cl *http.Client
}

var _ Client = &DirectClient{}

// NewDirectClient creates a DirectClient. If cl is nil, a default HTTP client is used.
func NewDirectClient(cl *http.Client) *DirectClient {
	if cl == nil {
		cl = defaultHTTPClient()
	}
	return &DirectClient{cl}
}

// Download downloads a file on GCS directly from storage.googleapis.com.
func (c *DirectClient) Download(ctx context.Context, w io.Writer, gsURL string) error {
	bucket, path, err := parseGSURL(gsURL)
	if err != nil {
		return err
	}

	dlURL := url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   fmt.Sprintf("/%s/%s", bucket, path),
	}
	req, err := http.NewRequest("GET", dlURL.String(), nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("got status %d", res.StatusCode)
	}

	_, err = io.Copy(w, res.Body)
	return err
}
