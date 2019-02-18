// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"io"
	"os"
)

// FakeClient is a fake implementation of devserver.Client suitable for unit tests.
type FakeClient struct {
	files map[string][]byte // GS URL -> content
}

var _ Client = &FakeClient{}

// NewFakeClient constructs a FakeClient. files is a map from GS URL to content.
func NewFakeClient(files map[string][]byte) *FakeClient {
	return &FakeClient{files}
}

// DownloadGS simulates a download from Google Cloud Storage.
func (c *FakeClient) DownloadGS(ctx context.Context, w io.Writer, gsURL string) (size int64, err error) {
	data, ok := c.files[gsURL]
	if !ok {
		return 0, os.ErrNotExist
	}
	n, err := w.Write(data)
	return int64(n), err
}
