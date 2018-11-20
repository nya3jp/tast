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
