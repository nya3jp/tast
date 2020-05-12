// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"io"
	"sync"
	"time"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/logging"
)

// CloudStorage allows Tast tests to read files on Google Cloud Storage.
type CloudStorage struct {
	// newClient is called to construct devserver.Client on the first call of Open lazily.
	// This is usually newClientForURLs, but might be different in unit tests.
	newClient func(ctx context.Context) devserver.Client

	once sync.Once
	cl   devserver.Client
}

// NewCloudStorage constructs a new CloudStorage from a list of Devserver URLs.
// This function is for the framework; tests should call testing.State.CloudStorage
// to get an instance.
func NewCloudStorage(devservers []string) *CloudStorage {
	return &CloudStorage{
		newClient: func(ctx context.Context) devserver.Client {
			return newClientForURLs(ctx, devservers)
		},
	}
}

// Open opens a file on Google Cloud Storage for read. Callers are responsible for
// closing the returned io.ReadCloser.
func (c *CloudStorage) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	c.once.Do(func() {
		c.cl = c.newClient(ctx)
	})
	return c.cl.Open(ctx, url)
}

func newClientForURLs(ctx context.Context, urls []string) devserver.Client {
	if len(urls) == 0 {
		logging.ContextLog(ctx, "Warning: Directly accessing Cloud Storage files because no devserver is available (using old tast command?)")
		return devserver.NewPseudoClient(nil)
	}

	const timeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return devserver.NewRealClient(ctx, urls, nil)
}
