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
	cl  *http.Client
	url string
}

var _ Client = &PseudoClient{}

// NewPseudoClient creates a PseudoClient. If cl is nil, a default HTTP client is used.
func NewPseudoClient(cl *http.Client) *PseudoClient {
	if cl == nil {
		cl = defaultHTTPClient
	}
	return &PseudoClient{cl: cl, url: gsDownloadURL}
}

// DownloadGS downloads a file on GCS directly from storage.googleapis.com.
func (c *PseudoClient) DownloadGS(ctx context.Context, w io.Writer, gsURL string) (size int64, err error) {
	bucket, path, err := parseGSURL(gsURL)
	if err != nil {
		return 0, err
	}

	dlURL, _ := url.Parse(c.url)
	dlURL.Path = fmt.Sprintf("/%s/%s", bucket, path)
	req, err := http.NewRequest("GET", dlURL.String(), nil)
	if err != nil {
		return 0, err
	}
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return io.Copy(w, res.Body)
	case http.StatusNotFound:
		return 0, os.ErrNotExist
	default:
		return 0, fmt.Errorf("got status %d", res.StatusCode)
	}
}
