// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/protocol"
)

// downloader encapsulates the logic to download external data files.
type downloader struct {
	m *extdata.Manager

	pcfg           *Config
	cl             devserver.Client
	beforeDownload func(context.Context)
}

func newDownloader(ctx context.Context, pcfg *Config) (*downloader, error) {
	cl, err := devserver.NewClient(
		ctx,
		pcfg.Service.GetDevservers(),
		pcfg.Service.GetTlwServer(),
		pcfg.Service.GetTlwSelfName(),
		pcfg.Service.GetDutServer(),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new client [devservers=%v, TLWServer=%s]",
			pcfg.Service.GetDevservers(), pcfg.Service.GetTlwServer())
	}
	m, err := extdata.NewManager(ctx, pcfg.Dirs.GetDataDir(), pcfg.DataFile.GetBuildArtifactsUrl())
	if err != nil {
		return nil, err
	}
	return &downloader{
		m:              m,
		pcfg:           pcfg,
		cl:             cl,
		beforeDownload: pcfg.BeforeDownload,
	}, nil
}

// TearDown must be called when downloader is destructed.
func (d *downloader) TearDown() error {
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
		extdata.RunDownloads(ctx, d.pcfg.Dirs.GetDataDir(), jobs, d.cl)
	}
	return release
}
