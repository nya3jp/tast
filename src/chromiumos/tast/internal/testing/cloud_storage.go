// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"io"
	"net/url"
	"strings"
	"sync"

	"chromiumos/tast/internal/devserver"
)

// CloudStorage allows Tast tests to read files on Google Cloud Storage.
type CloudStorage struct {
	// newClient is called to construct devserver.Client on the first call of Open lazily.
	// This is usually newClientForURLs, but might be different in unit tests.
	newClient func(ctx context.Context) (devserver.Client, error)

	once              sync.Once
	cl                devserver.Client
	initErr           error
	buildArtifactsURL string
}

// NewCloudStorage constructs a new CloudStorage from a list of Devserver URLs.
// This function is for the framework; tests should call testing.State.CloudStorage
// to get an instance.
func NewCloudStorage(devservers []string, tlwServer, dutName, dutServer, buildArtifactsURL string) *CloudStorage {
	return &CloudStorage{
		newClient: func(ctx context.Context) (devserver.Client, error) {
			return newClientForURLs(ctx, devservers, tlwServer, dutName, dutServer)
		},
		buildArtifactsURL: buildArtifactsURL,
	}
}

// Open opens a file on Google Cloud Storage for read. Callers are responsible for
// closing the returned io.ReadCloser.
// Urls that start with build-artifact:/// will be treated as relative to the BuildArtifactsURL
// passed to tast in the --buildartifactsurl argument.
func (c *CloudStorage) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	c.once.Do(func() {
		c.cl, c.initErr = c.newClient(ctx)
	})
	if c.initErr != nil {
		return nil, c.initErr
	}
	buildArtifactPrefix := "build-artifact:///"
	if strings.HasPrefix(url, buildArtifactPrefix) {
		url = c.buildArtifactsURL + strings.TrimPrefix(url, buildArtifactPrefix)
	}
	return c.cl.Open(ctx, url)
}

// Stage opens a file on Google Cloud Storage for read. Returns a http or file url to the data.
// Urls that start with build-artifact:/// will be treated as relative to the BuildArtifactsURL
// passed to tast in the --buildartifactsurl argument.
func (c *CloudStorage) Stage(ctx context.Context, url string) (*url.URL, error) {
	c.once.Do(func() {
		c.cl, c.initErr = c.newClient(ctx)
	})
	if c.initErr != nil {
		return nil, c.initErr
	}
	buildArtifactPrefix := "build-artifact:///"
	if strings.HasPrefix(url, buildArtifactPrefix) {
		url = c.buildArtifactsURL + strings.TrimPrefix(url, buildArtifactPrefix)
	}
	return c.cl.Stage(ctx, url)
}

func newClientForURLs(ctx context.Context, urls []string, tlwServer, dutName, dutServer string) (devserver.Client, error) {
	return devserver.NewClient(ctx, urls, tlwServer, dutName, dutServer)
}
