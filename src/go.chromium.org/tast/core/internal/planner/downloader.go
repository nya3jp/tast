// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"io"
	"net/url"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/devserver"
	"go.chromium.org/tast/core/internal/extdata"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
)

// downloader encapsulates the logic to download external data files.
type downloader struct {
	m *extdata.Manager

	pcfg           *Config
	cl             devserver.Client
	beforeDownload func(context.Context)
}

func newDownloader(ctx context.Context, pcfg *Config) (*downloader, error) {
	m, err := extdata.NewManager(ctx, pcfg.Dirs.GetDataDir(), pcfg.DataFile.GetBuildArtifactsUrl())
	if err != nil {
		return nil, err
	}
	return &downloader{
		m:              m,
		pcfg:           pcfg,
		beforeDownload: pcfg.BeforeDownload,
	}, nil
}

// TearDown must be called when downloader is destructed.
func (d *downloader) TearDown() error {
	if d.cl == nil {
		return nil
	}
	return d.cl.TearDown()
}

// BeforeRun must be called before running a set of tests. It downloads external
// data files if Config.DownloadMode is DownloadBatch.
func (d *downloader) BeforeRun(ctx context.Context, tests []*protocol.Entity) {
	if d.pcfg.DataFile.GetDownloadMode() == protocol.DownloadMode_BATCH {
		// Ignore release because no data files are to be purged.
		d.download(ctx, tests)
	}
}

// BeforeEntity must be called before running each entity. It downloads external
// data files if Config.DownloadMode is DownloadLazy.
//
// release must be called after entity finishes.
func (d *downloader) BeforeEntity(ctx context.Context, entity *protocol.Entity) (release func()) {
	if d.pcfg.DataFile.GetDownloadMode() == protocol.DownloadMode_LAZY {
		return d.download(ctx, []*protocol.Entity{entity})
	}
	return func() {}
}

func (d *downloader) download(ctx context.Context, entities []*protocol.Entity) (release func()) {
	jobs, release := d.m.PrepareDownloads(ctx, entities)
	if len(jobs) > 0 {
		if d.beforeDownload != nil {
			d.beforeDownload(ctx)
		}
		extdata.RunDownloads(ctx, d.pcfg.Dirs.GetDataDir(), jobs, d.client(ctx))
	}
	return release
}

// client returns a cached devserver.Client, creating it on first download so
// runs that need zero downloads do not issue a /check_health HTTP request.
func (d *downloader) client(ctx context.Context) devserver.Client {
	if d.cl != nil {
		return d.cl
	}
	cl, err := devserver.NewClient(
		ctx,
		d.pcfg.Service.GetDevservers(),
		d.pcfg.Service.GetTlwServer(),
		d.pcfg.Service.GetTlwSelfName(),
		d.pcfg.Service.GetDutServer(),
		d.pcfg.Service.GetSwarmingTaskID(),
		d.pcfg.Service.GetBuildBucketID(),
	)
	if err != nil {
		err = errors.Wrapf(err, "failed to create new client [devservers=%v, TLWServer=%s]",
			d.pcfg.Service.GetDevservers(), d.pcfg.Service.GetTlwServer())
		logging.Infof(ctx, "Failed to create devserver client: %v", err)
		// Pass an error-returning client to RunDownloads so every job records a
		// .external-error file via standard external-data failure reporting.
		cl = &errClient{err: err}
	}
	d.cl = cl
	return d.cl
}

type errClient struct {
	err error
}

var _ devserver.Client = (*errClient)(nil)

func (c *errClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	return nil, c.err
}

func (c *errClient) Stage(ctx context.Context, gsURL string) (*url.URL, error) {
	return nil, c.err
}

func (c *errClient) TearDown() error {
	return nil
}
