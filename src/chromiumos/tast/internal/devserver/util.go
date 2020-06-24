// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// defaultHTTPClient is a HTTP client used by default by devserver clients.
// It is different from net/http's default client since we want more concurrent
// connections for parallel downloads.
var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 10,
		Proxy:               http.ProxyFromEnvironment,
	},
}

// ParseGSURL parses a Google Cloud Storage URL. It is parsed as:
//
//  gs://<bucket>/<path>
//
// Note that path is not prefixed with a slash, which is suitable for use with
// GCS APIs.
func ParseGSURL(gsURL string) (bucket, path string, err error) {
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
