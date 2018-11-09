// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package storage provides a client for Google Cloud Storage, possibly using
// devservers to reduce traffic.
package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is an abstract client interface for downloading files from GCS.
type Client interface {
	// Download downloads a file at gsURL which must start with gs://.
	Download(ctx context.Context, w io.Writer, gsURL string) error
}

func defaultHTTPClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConnsPerHost: 10,
	}
	return &http.Client{
		Transport: tr,
	}
}

func parseGSURL(gsURL string) (bucket, path string, err error) {
	parsed, err := url.Parse(gsURL)
	if err != nil {
		return "", "", err
	}
	if parsed.Scheme != "gs" {
		return "", "", errors.New("not a GS URL")
	}

	bucket = parsed.Host
	path = strings.TrimPrefix(parsed.Path, "/")
	return bucket, path, nil
}
