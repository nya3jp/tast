// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
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

// Open simulates a download from Google Cloud Storage.
func (c *FakeClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	data, ok := c.files[gsURL]
	if !ok {
		return nil, os.ErrNotExist
	}
	return ioutil.NopCloser(bytes.NewReader(data)), nil
}
